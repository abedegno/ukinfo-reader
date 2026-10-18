// Package desktop owns a private PostgreSQL child process. It never adopts or
// terminates an existing postmaster, deletes a cluster, or upgrades one in place.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/jackc/pgx/v5"
)

type Local struct {
	URL  string
	cmd  *exec.Cmd
	lock *flock.Flock
	done chan struct{}
	log  io.WriteCloser
	once sync.Once
}

func (p *Local) Close() {
	p.once.Do(func() {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(os.Interrupt)
			select {
			case <-p.done:
			case <-time.After(15 * time.Second):
				_ = p.cmd.Process.Kill()
				<-p.done
			}
		}
		if p.log != nil {
			_ = p.log.Close()
		}
		if p.lock != nil {
			_ = p.lock.Unlock()
		}
	})
}

func Start(ctx context.Context, directory, bin string) (_ *Local, err error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("embedded PostgreSQL on Windows is not supported yet; use DATABASE_URL")
	}
	if bin == "" {
		return nil, fmt.Errorf("--pg-bin must contain PostgreSQL 18 binaries (no runtime downloads)")
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return nil, err
	}
	version, err := exec.CommandContext(ctx, filepath.Join(bin, "postgres"), "--version").Output()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(string(version)), "postgres (PostgreSQL) 18.") {
		return nil, fmt.Errorf("PostgreSQL 18 binaries required")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	p := &Local{lock: flock.New(filepath.Join(directory, "owner.lock"))}
	locked, e := p.lock.TryLock()
	if e != nil {
		return nil, e
	}
	if !locked {
		return nil, fmt.Errorf("data directory is already owned by another app instance")
	}
	defer func() {
		if err != nil {
			p.Close()
		}
	}()
	passwordPath := filepath.Join(directory, "password")
	password, e := os.ReadFile(passwordPath)
	if os.IsNotExist(e) {
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return nil, err
		}
		password = []byte(hex.EncodeToString(secret))
		if err = os.WriteFile(passwordPath, password, 0600); err != nil {
			return nil, err
		}
	} else if e != nil {
		return nil, e
	}
	if len(password) != 64 {
		return nil, fmt.Errorf("invalid local credential file; preserve the cluster and restore its original password")
	}
	data := filepath.Join(directory, "pg18")
	if existing, e := os.ReadFile(filepath.Join(data, "PG_VERSION")); e == nil {
		if strings.TrimSpace(string(existing)) != "18" {
			return nil, fmt.Errorf("cluster major version differs; use a logical export/import")
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	} else {
		// initdb refuses nonempty directories. Never clear them on startup failure.
		initCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		cmd := exec.CommandContext(initCtx, filepath.Join(bin, "initdb"), "-D", data, "--username=ukinfo", "--pwfile="+passwordPath, "--auth=scram-sha-256", "--encoding=UTF8", "--locale=C")
		out, e := cmd.CombinedOutput()
		cancel()
		if e != nil {
			return nil, fmt.Errorf("initialize PostgreSQL: %w: %s", e, out)
		}
	}
	if _, e = os.Stat(filepath.Join(data, "postmaster.pid")); e == nil {
		// pg_ctl only inspects status here. PostgreSQL itself owns stale-lock and
		// shared-memory checks during startup; the app never deletes PID files.
		check, cancel := context.WithTimeout(ctx, 5*time.Second)
		statusErr := exec.CommandContext(check, filepath.Join(bin, "pg_ctl"), "status", "-D", data).Run()
		cancel()
		var exit *exec.ExitError
		if !errors.As(statusErr, &exit) || exit.ExitCode() != 3 {
			return nil, fmt.Errorf("archive database may still be running; close its owning app before retrying (files preserved)")
		}
	} else if !os.IsNotExist(e) {
		return nil, fmt.Errorf("cannot inspect archive database status; files preserved")
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return nil, e
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	p.log, err = openRotatingLog(filepath.Join(directory, "postgres.log"), 8<<20, 3)
	if err != nil {
		return nil, err
	}
	p.cmd = exec.Command(filepath.Join(bin, "postgres"), "-D", data, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-k", "", "-c", "shared_buffers=128MB", "-c", "max_connections=24")
	p.cmd.Stdout = p.log
	p.cmd.Stderr = p.log
	p.done = make(chan struct{})
	if err = p.cmd.Start(); err != nil {
		p.cmd = nil
		return nil, err
	}
	go func() { _ = p.cmd.Wait(); close(p.done) }()
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "postgres", User: url.UserPassword("ukinfo", string(password)), RawQuery: "sslmode=disable"}
	p.URL = u.String()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		attempt, done := context.WithTimeout(startup, time.Second)
		db, e := pgx.Connect(attempt, p.URL)
		if e == nil {
			e = db.Ping(attempt)
			_ = db.Close(context.Background())
		}
		done()
		if e == nil {
			return p, nil
		}
		select {
		case <-p.done:
			return nil, fmt.Errorf("PostgreSQL exited; inspect postgres.log")
		case <-startup.Done():
			return nil, fmt.Errorf("PostgreSQL startup timed out; inspect postgres.log")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
