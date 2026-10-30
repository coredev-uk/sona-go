package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// Defaults for [Options].
const (
	DefaultMaxBytes = 5 << 20 // 5 MiB per file
	DefaultBackups  = 3
)

// Options configures [New].
type Options struct {
	// File is the JSON log file (normally config.Paths.LogFile()). Empty
	// disables file logging.
	File string
	// MaxBytes rotates File once a write would exceed it. Zero means
	// DefaultMaxBytes.
	MaxBytes int64
	// Backups is how many rotated files (File.1 … File.N) are kept. Zero
	// means DefaultBackups.
	Backups int
	// Dev adds a human-readable text handler on Stderr at debug level.
	Dev bool
	// Stderr overrides os.Stderr for the dev handler.
	Stderr io.Writer
	// Level is the file handler's minimum level; nil means slog.LevelInfo.
	Level slog.Leveler
}

// New builds the application logger. Every handler sits behind a
// [RedactHandler]. The returned close function flushes and closes the log
// file; call it on shutdown.
func New(opts Options) (*slog.Logger, func() error, error) {
	var handlers []slog.Handler
	closeFn := func() error { return nil }

	if opts.File != "" {
		f, err := OpenRotatingFile(opts.File, opts.MaxBytes, opts.Backups)
		if err != nil {
			return nil, nil, err
		}
		closeFn = f.Close
		level := opts.Level
		if level == nil {
			level = slog.LevelInfo
		}
		handlers = append(handlers, slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level}))
	}
	if opts.Dev {
		w := opts.Stderr
		if w == nil {
			w = os.Stderr
		}
		handlers = append(handlers, slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}

	var h slog.Handler
	switch len(handlers) {
	case 0:
		h = slog.DiscardHandler
	case 1:
		h = handlers[0]
	default:
		h = fanout(handlers)
	}
	return slog.New(NewRedactHandler(h)), closeFn, nil
}

// fanout sends each record to every handler that accepts its level.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var errs []error
	for _, h := range f {
		if h.Enabled(ctx, r.Level) {
			errs = append(errs, h.Handle(ctx, r.Clone()))
		}
	}
	return errors.Join(errs...)
}

func (f fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithAttrs(attrs)
	}
	return out
}

func (f fanout) WithGroup(name string) slog.Handler {
	out := make(fanout, len(f))
	for i, h := range f {
		out[i] = h.WithGroup(name)
	}
	return out
}

// RotatingFile is an append-only 0600 file that rotates by size: when a
// write would push it past its limit, path.N-1 → path.N … path → path.1 and
// a fresh path is opened. The oldest backup beyond the limit is deleted.
type RotatingFile struct {
	path     string
	maxBytes int64
	backups  int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// OpenRotatingFile opens (appending to) the file at path. Non-positive
// maxBytes and backups select the defaults.
func OpenRotatingFile(path string, maxBytes int64, backups int) (*RotatingFile, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if backups <= 0 {
		backups = DefaultBackups
	}
	r := &RotatingFile{path: path, maxBytes: maxBytes, backups: backups}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("logging: open %s: %w", r.path, err)
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("logging: stat %s: %w", r.path, err)
	}
	r.f, r.size = f, fi.Size()
	return nil
}

// Write implements io.Writer. A single write larger than the limit still
// lands whole in a fresh file.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) rotate() error {
	if err := r.f.Close(); err != nil {
		return fmt.Errorf("logging: close for rotation: %w", err)
	}
	r.f = nil
	_ = os.Remove(fmt.Sprintf("%s.%d", r.path, r.backups))
	for i := r.backups - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	if err := os.Rename(r.path, r.path+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("logging: rotate %s: %w", r.path, err)
	}
	return r.open()
}

// Close syncs and closes the file. Further writes fail with os.ErrClosed.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := errors.Join(r.f.Sync(), r.f.Close())
	r.f = nil
	return err
}
