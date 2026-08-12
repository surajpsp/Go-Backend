package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go-backend/internal/logger"
	"go-backend/internal/router"
	"go-backend/internal/seed"
	"go-backend/internal/store"
)

func main() {
	// Exit code goes through run() so every deferred close still happens;
	// os.Exit from inside would skip them and truncate the log file.
	os.Exit(run())
}

func run() int {
	var (
		addr     = flag.String("addr", envOr("ADDR", ":8080"), "host:port to listen on")
		dbPath   = flag.String("db", envOr("DB_PATH", "products.db"), "path to the SQLite database file")
		logDir   = flag.String("log-dir", envOr("LOG_DIR", "logs"), "directory for the daily JSON log files")
		seedFlag = flag.Bool("seed", false, "insert demo products before starting (skips existing SKUs)")
		seedOnly = flag.Bool("seed-only", false, "insert demo products and exit without starting the server")
	)
	flag.Parse()

	logFile, err := logger.Init(*logDir)
	if err != nil {
		log.Printf("failed to init logger: %v", err)
		return 1
	}
	defer logFile.Close()

	s, err := store.NewSQLite(*dbPath)
	if err != nil {
		slog.Error("failed to init store", "err", err, "db", *dbPath)
		return 1
	}
	defer s.Close()

	if *seedFlag || *seedOnly {
		n, err := seed.Run(s)
		if err != nil {
			slog.Error("seed failed", "err", err, "inserted", n)
			return 1
		}
		slog.Info("seed complete", "inserted", n, "skipped", seed.Total()-n)
		if *seedOnly {
			return 0
		}
	}

	return serve(*addr, router.New(s))
}

// serve runs the HTTP server until SIGINT/SIGTERM, then drains in-flight
// requests before returning so a Ctrl-C doesn't cut a response in half or leave
// the database and log file mid-write.
func serve(addr string, h http.Handler) int {
	srv := &http.Server{
		Addr:    addr,
		Handler: h,
		// Bounds on a slow or stalled peer, so one connection can't hold a
		// goroutine and a file descriptor open indefinitely.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server starting", "addr", addr)
		// ErrServerClosed is the normal result of Shutdown, not a failure.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			slog.Error("server failed", "err", err)
			return 1
		}
		return 0
	case sig := <-stop:
		slog.Info("shutdown signal received", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		return 1
	}
	slog.Info("server stopped")
	return 0
}

// envOr returns the value of the environment variable key, or def when unset.
// Flags still win: they are parsed with these as their defaults.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
