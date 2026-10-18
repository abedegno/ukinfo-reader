// ukinfo-server serves the same catalogue and UI in hosted and desktop modes.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/abedegno/ukinfo-reader/server/internal/desktop"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed web/*
var assets embed.FS

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	command := "serve"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		command = os.Args[1]
		os.Args = append(os.Args[:1], os.Args[2:]...)
	}
	flags := flag.NewFlagSet("ukinfo-server", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP listen address; :0 selects an available port")
	metricsListen := flags.String("metrics-listen", "", "optional private loopback metrics listener (127.0.0.1:9091)")
	origin := flags.String("public-origin", "", "public origin behind a TLS reverse proxy (https://example.org)")
	data := flags.String("data-dir", "", "desktop: app-managed data directory")
	binaries := flags.String("pg-bin", "", "desktop: directory containing PostgreSQL 18 initdb and postgres")
	desktopControl := flags.Bool("desktop-control", false, "enable capability-protected desktop ingestion on a separate loopback port")
	legacyData := flags.String("source-data-dir", "", "repack-v1: app-managed schema-1 source directory; it remains unchanged")
	addressBlocks := flags.Int("address-blocks", 0, "index-addresses: rollback-only pilot with this many evenly spaced source blocks per edition; zero builds the full index")
	datasetKey := flags.String("dataset", "", "dataset key for verify index-addresses or index-research")
	onspd := flags.String("onspd-zip", "", "import-onspd: local official ZIP (no extraction)")
	geoRelease := flags.String("release", catalog.ONSPDRelease, "ONSPD release YYYY-MM")
	geoSHA := flags.String("sha256", catalog.ONSPDSHA256, "expected ONSPD ZIP SHA-256")
	geoURL := flags.String("source-url", catalog.ONSPDURL, "official ONSPD source URL")
	parent := flags.Bool("parent-stdin", false, "desktop: shut down when parent closes stdin")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if command != "serve" && command != "migrate" && command != "import" && command != "verify" && command != "repack-v1" && command != "stats" && command != "import-onspd" && command != "index-addresses" && command != "lock-archive" && command != "index-research" {
		return fmt.Errorf("command must be serve, migrate, import, verify, stats, repack-v1, import-onspd index-addresses or index-research")
	}
	if *desktopControl {
		host, _, e := net.SplitHostPort(*listen)
		if command != "serve" || e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("desktop-control requires serve on an explicit loopback address")
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *parent {
		if command != "serve" {
			return fmt.Errorf("parent-stdin requires serve")
		}
		go func() { _, _ = io.Copy(io.Discard, os.Stdin); stop() }()
	}
	if command == "lock-archive" {
		if *data == "" {
			return fmt.Errorf("lock-archive requires --data-dir")
		}
		return desktop.HoldArchiveLock(ctx, *data, os.Stdin, os.Stdout)
	}
	dsn := os.Getenv("DATABASE_URL")
	if *data != "" {
		if dsn != "" {
			return fmt.Errorf("choose DATABASE_URL or data-dir")
		}
		local, err := desktop.Start(ctx, *data, *binaries)
		if err != nil {
			return err
		}
		defer local.Close()
		dsn = local.URL
	}
	if dsn == "" {
		return fmt.Errorf("set DATABASE_URL or use --data-dir with --pg-bin")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid database configuration")
	}
	config.MaxConns = 8
	config.MinConns = 1
	config.ConnConfig.ConnectTimeout = 5 * time.Second
	config.ConnConfig.RuntimeParams["statement_timeout"] = "2000"
	if command != "serve" {
		config.ConnConfig.RuntimeParams["statement_timeout"] = "0"
	}
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("database configuration failed")
	}
	defer db.Close()
	if command == "migrate" || command == "import-onspd" || (command == "index-addresses" || command == "index-research") || *data != "" {
		if err = catalog.Migrate(ctx, db); err != nil {
			return err
		}
		if command == "migrate" {
			return nil
		}
	}
	if command == "import-onspd" {
		if *onspd == "" {
			return fmt.Errorf("import-onspd requires --onspd-zip")
		}
		report, e := catalog.ImportGeography(ctx, db, *onspd, catalog.GeographyImportOptions{Release: *geoRelease, SHA256: *geoSHA, SourceURL: *geoURL, CheckResources: func() error {
			for _, path := range []string{*onspd, os.TempDir()} {
				free, err := desktop.AvailableBytes(path)
				if err != nil {
					return err
				}
				if free < 5<<30 {
					return catalog.ErrDiskHeadroom
				}
			}
			return nil
		}, Progress: func(p catalog.Progress) { log.Printf("%s: %d", p.Phase, p.Done) }})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	var lastLog time.Time
	var lastPhase string
	progress := func(p catalog.Progress) {
		if p.Phase != lastPhase || time.Since(lastLog) > 5*time.Second {
			log.Printf("%s: %d / %d", p.Phase, p.Done, p.Total)
			lastLog = time.Now()
			lastPhase = p.Phase
		}
	}
	checkAddressResources := func() error {
		free, e := desktop.AvailableBytes(os.TempDir())
		if e != nil {
			return e
		}
		if free < 5<<30 {
			return catalog.ErrDiskHeadroom
		}
		return nil
	}
	if command == "import" {
		n, e := catalog.ImportWithOptions(ctx, db, os.Stdin, catalog.ImportOptions{Progress: progress, IndexAddresses: true, CheckResources: checkAddressResources})
		if e != nil {
			return e
		}
		fmt.Fprintf(os.Stderr, "Imported %d entries\n", n)
		return nil
	}
	if command == "index-research" {
		if *datasetKey == "" {
			return errors.New("index-research requires --dataset")
		}
		report, e := catalog.IndexResearch(ctx, db, *datasetKey, catalog.AddressIndexOptions{CheckResources: checkAddressResources, Progress: progress})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if command == "index-addresses" {
		if *datasetKey == "" {
			return fmt.Errorf("index-addresses requires --dataset")
		}
		report, e := catalog.IndexAddresses(ctx, db, *datasetKey, catalog.AddressIndexOptions{SampleBlocks: *addressBlocks, Progress: progress, CheckResources: checkAddressResources})
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if command == "stats" {
		raw, e := catalog.Statistics(ctx, db)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(raw)
	}
	if command == "verify" {
		if *datasetKey == "" {
			return fmt.Errorf("verify requires --dataset")
		}
		report, e := catalog.Verify(ctx, db, *datasetKey, progress)
		if e != nil {
			return e
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if command == "repack-v1" {
		sourceURL := os.Getenv("SOURCE_DATABASE_URL")
		if *legacyData != "" {
			if sourceURL != "" {
				return fmt.Errorf("choose SOURCE_DATABASE_URL or source-data-dir")
			}
			local, e := desktop.Start(ctx, *legacyData, *binaries)
			if e != nil {
				return e
			}
			defer local.Close()
			sourceURL = local.URL
		}
		if sourceURL == "" {
			return fmt.Errorf("set SOURCE_DATABASE_URL to a separate schema-1 source")
		}
		sourceConfig, e := pgxpool.ParseConfig(sourceURL)
		if e != nil {
			return fmt.Errorf("invalid source database configuration")
		}
		sourceConfig.MaxConns = 2
		sourceConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		sourceConfig.ConnConfig.RuntimeParams["statement_timeout"] = "0"
		source, e := pgxpool.NewWithConfig(ctx, sourceConfig)
		if e != nil {
			return e
		}
		defer source.Close()
		return catalog.RepackV1(ctx, db, source, catalog.ImportOptions{Progress: progress})
	}
	if err = db.Ping(ctx); err != nil {
		return fmt.Errorf("database unavailable")
	}
	var version int
	if err = db.QueryRow(ctx, "SELECT max(version) FROM catalog_schema").Scan(&version); err != nil || version != catalog.SchemaVersion {
		return fmt.Errorf("catalogue schema unavailable; run migrate with an owner connection")
	}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	localOrigin := "http://" + listener.Addr().String()
	allowedOrigin := localOrigin
	if *origin != "" {
		allowedOrigin = strings.TrimRight(*origin, "/")
	}
	tileURL := os.Getenv("UKINFO_MAP_TILES_URL")
	if tileURL == "" {
		tileURL = "https://tile.openstreetmap.org/{z}/{x}/{y}.png"
	}
	tileOrigin := ""
	if tileURL != "none" {
		parsed, e := url.Parse(tileURL)
		if e != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || !strings.Contains(tileURL, "{z}") || !strings.Contains(tileURL, "{x}") || !strings.Contains(tileURL, "{y}") {
			return errors.New("UKINFO_MAP_TILES_URL needs an HTTPS {z}/{x}/{y} template, or none")
		}
		tileOrigin = parsed.Scheme + "://" + parsed.Host
	}
	tileAttribution := os.Getenv("UKINFO_MAP_ATTRIBUTION")
	if tileAttribution == "" {
		tileAttribution = "© OpenStreetMap contributors"
	}
	ui, _ := fs.Sub(assets, "web")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/map-config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"tiles_url": tileURL, "attribution": tileAttribution})
	})
	api := catalog.NewAPI(db)
	healthConfig := config.Copy()
	healthConfig.MaxConns, healthConfig.MinConns = 1, 0
	healthConfig.ConnConfig.RuntimeParams["statement_timeout"] = "1000"
	healthDB, e := pgxpool.NewWithConfig(ctx, healthConfig)
	if e != nil {
		return errors.New("health database configuration failed")
	}
	defer healthDB.Close()
	api.HealthDB = healthDB
	if *metricsListen != "" {
		host, _, e := net.SplitHostPort(*metricsListen)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return errors.New("metrics-listen requires an explicit loopback IP address")
		}
		metricsListener, e := net.Listen("tcp", *metricsListen)
		if e != nil {
			return e
		}
		metricsServer := &http.Server{Handler: api.Metrics(), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
		defer metricsServer.Close()
		go func() { _ = metricsServer.Serve(metricsListener) }()
		go func() { <-ctx.Done(); _ = metricsServer.Close() }()
	}
	mux.Handle("/api/", api.Handler())
	mux.Handle("/", http.FileServer(http.FS(ui)))
	secure := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject cross-origin requests and DNS rebinding against the local server.
		if r.Host != strings.TrimPrefix(strings.TrimPrefix(allowedOrigin, "https://"), "http://") {
			http.Error(w, "Invalid host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != allowedOrigin {
			http.Error(w, "Invalid origin", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data: "+tileOrigin+"; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	})
	server := &http.Server{Handler: secure, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	// Capabilities go only to the owning desktop main process, never the renderer.
	boot := map[string]string{"url": localOrigin}
	if *desktopControl {
		controlListener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			return e
		}
		secret := make([]byte, 32)
		if _, e = rand.Read(secret); e != nil {
			_ = controlListener.Close()
			return e
		}
		token := hex.EncodeToString(secret)
		admin := &catalog.Admin{DB: db, Token: token, Host: controlListener.Addr().String()}
		if *data != "" {
			admin.CheckResources = func() error {
				for _, path := range []string{*data, os.TempDir()} {
					free, e := desktop.AvailableBytes(path)
					if e != nil {
						return e
					}
					if free < 5<<30 {
						return catalog.ErrDiskHeadroom
					}
				}
				return nil
			}
		}
		control := &http.Server{Handler: admin.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16384}
		defer control.Close()
		go func() { _ = control.Serve(controlListener) }()
		go func() { <-ctx.Done(); admin.Cancel(); _ = control.Close() }()
		boot["control_url"] = "http://" + controlListener.Addr().String()
		boot["control_token"] = token
	}
	if err = json.NewEncoder(os.Stdout).Encode(boot); err != nil {
		return err
	}
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
