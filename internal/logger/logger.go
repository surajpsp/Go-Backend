// Package logger sets up the application-wide structured logger: pretty text on
// the console for a human watching the terminal, and one JSON line per event in
// a date-stamped file for anything you need to grep later.
package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Init creates dir if needed and installs the default slog logger. The returned
// Closer flushes and closes the current log file; call it on shutdown.
func Init(dir string) (io.Closer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	file := &dailyFile{dir: dir}

	handler := fanout{handlers: []slog.Handler{
		// Console: readable at a glance, no timestamp noise beyond the default.
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
		// File: JSON lines, machine-parseable.
		slog.NewJSONHandler(file, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}}
	slog.SetDefault(slog.New(handler))
	return file, nil
}

// dailyFile appends to <dir>/app-YYYY-MM-DD.log and swaps to a new file when the
// date rolls over, so logs rotate daily without a third-party dependency.
type dailyFile struct {
	dir string

	mu  sync.Mutex // guards day and f
	day string
	f   *os.File
}

func (d *dailyFile) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	if d.f == nil || day != d.day {
		if d.f != nil {
			d.f.Close()
		}
		f, err := os.OpenFile(
			filepath.Join(d.dir, "app-"+day+".log"),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644,
		)
		if err != nil {
			return 0, err
		}
		d.f, d.day = f, day
	}
	return d.f.Write(p)
}

func (d *dailyFile) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.f == nil {
		return nil
	}
	err := d.f.Close()
	d.f = nil
	return err
}

// fanout sends every record to each wrapped handler, so one slog call lands on
// the console and in the file at the same time.
type fanout struct{ handlers []slog.Handler }

func (f fanout) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range f.handlers {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		// Clone: a handler may retain or mutate the record it is given.
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return fanout{handlers: next}
}

func (f fanout) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(f.handlers))
	for i, h := range f.handlers {
		next[i] = h.WithGroup(name)
	}
	return fanout{handlers: next}
}
