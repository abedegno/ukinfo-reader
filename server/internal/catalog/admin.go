package catalog

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ImportState struct {
	Running  bool     `json:"running"`
	Progress Progress `json:"progress"`
	Error    string   `json:"error,omitempty"`
}

// Admin is registered only on an ephemeral loopback listener in explicit desktop
// mode. Its random capability is held by Electron main, never the web renderer.
var ErrDiskHeadroom = errors.New("retain at least 5 GiB of free disk space; import stopped")

type Admin struct {
	CheckResources func() error
	DB             *pgxpool.Pool
	Token          string
	Host           string
	mu             sync.Mutex
	state          ImportState
	cancel         context.CancelFunc
}

func (a *Admin) Cancel() {
	a.mu.Lock()
	cancel := a.cancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
func (a *Admin) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /progress", func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		state := a.state
		a.mu.Unlock()
		respond(w, 200, state)
	})
	mux.HandleFunc("POST /cancel", func(w http.ResponseWriter, r *http.Request) {
		a.Cancel()
		respond(w, 202, map[string]string{"status": "canceling"})
	})
	mux.HandleFunc("POST /import", a.importStream)
	mux.HandleFunc("GET /geography-source", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"release": ONSPDRelease, "sha256": ONSPDSHA256, "url": ONSPDURL})
	})
	mux.HandleFunc("POST /import-geography", a.importGeography)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(a.Token) != 64 || r.Host != a.Host || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(token), []byte(a.Token)) != 1 {
			fail(w, 403, "Desktop capability required")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *Admin) importStream(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	_ = controller.EnableFullDuplex()
	if r.Header.Get("Content-Type") != "application/x-ndjson" {
		fail(w, 415, "Expected a verified catalogue stream")
		return
	}
	base, timeoutCancel := context.WithTimeout(r.Context(), 4*time.Hour)
	defer timeoutCancel()
	ctx, cancelCause := context.WithCancelCause(base)
	defer cancelCause(nil)
	cancel := func() { cancelCause(context.Canceled) }
	if a.CheckResources != nil {
		if err := a.CheckResources(); err != nil {
			_ = controller.SetReadDeadline(time.Now())
			fail(w, 507, err.Error())
			return
		}
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := a.CheckResources(); err != nil {
						cancelCause(err)
						return
					}
				}
			}
		}()
	}
	_ = controller.SetReadDeadline(time.Now().Add(4 * time.Hour))
	// Interrupt a stalled producer when canceled; canceling SQL alone cannot
	// interrupt Scanner.Scan while the request body is waiting for more bytes.
	stop := context.AfterFunc(ctx, func() { _ = controller.SetReadDeadline(time.Now()) })
	defer stop()
	a.mu.Lock()
	if a.state.Running {
		a.mu.Unlock()
		fail(w, 409, "An import is already running")
		return
	}
	a.state = ImportState{Running: true, Progress: Progress{Phase: "checking source"}}
	a.cancel = cancel
	a.mu.Unlock()
	options := ImportOptions{IndexAddresses: true, Progress: func(p Progress) { a.mu.Lock(); a.state.Progress = p; a.mu.Unlock() }}
	count, err := ImportWithOptions(ctx, a.DB, r.Body, options)
	a.mu.Lock()
	a.state.Running = false
	a.cancel = nil
	if err != nil {
		if cause := context.Cause(ctx); cause != nil {
			err = cause
		}
		log.Printf("Desktop import stopped: %v", err)
		a.state.Error = err.Error()
		a.state.Progress.Phase = "failed"
		if errors.Is(err, context.Canceled) {
			a.state.Progress.Phase = "canceled"
		}
	}
	a.mu.Unlock()
	if err != nil {
		_ = controller.SetReadDeadline(time.Now())
		if errors.Is(err, ErrDiskHeadroom) {
			fail(w, 507, "Not enough free disk space. Import stopped; check the catalogue before retrying.")
		} else {
			fail(w, 422, "Import stopped; check the catalogue before retrying")
		}
		return
	}
	respond(w, 200, map[string]int64{"entries": count})
}

// importGeography accepts a local path only from the owning native process. The
// renderer never receives the capability or an unrestricted file-path API.
func (a *Admin) importGeography(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Path string `json:"path"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
	decoder.DisallowUnknownFields()
	if r.Header.Get("Content-Type") != "application/json" || decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !filepath.IsAbs(input.Path) {
		fail(w, 400, "Choose a local ONSPD ZIP in the desktop app")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Hour)
	defer cancel()
	a.mu.Lock()
	if a.state.Running {
		a.mu.Unlock()
		fail(w, 409, "An import is already running")
		return
	}
	a.state = ImportState{Running: true, Progress: Progress{Phase: "verifying postcode archive"}}
	a.cancel = cancel
	a.mu.Unlock()
	report, err := ImportGeography(ctx, a.DB, input.Path, GeographyImportOptions{
		Release: ONSPDRelease, SHA256: ONSPDSHA256, SourceURL: ONSPDURL,
		CheckResources: a.CheckResources,
		Progress:       func(p Progress) { a.mu.Lock(); a.state.Progress = p; a.mu.Unlock() },
	})
	a.mu.Lock()
	a.state.Running = false
	a.cancel = nil
	if err != nil {
		a.state.Error = err.Error()
		a.state.Progress.Phase = "failed"
		if errors.Is(err, context.Canceled) {
			a.state.Progress.Phase = "canceled"
		}
	} else {
		a.state.Progress = Progress{Phase: "complete", Done: report.Entries, Total: report.Entries}
	}
	a.mu.Unlock()
	if err != nil {
		log.Printf("Desktop postcode import stopped: %v", err)
		status := 422
		if errors.Is(err, ErrDiskHeadroom) {
			status = 507
		}
		fail(w, status, err.Error())
		return
	}
	respond(w, 200, report)
}
