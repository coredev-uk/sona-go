// Package diag captures crashes and builds the redacted diagnostics bundle:
// fatal runtime output goes to a crash file in the state directory, panics
// in background goroutines are recovered and logged with their stack, and
// Bundle collects versions, settings, crash output and the log tail for a
// bug report with every secret removed.
package diag

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// Crash files in the state directory. CrashFile receives this run's fatal
// output (unrecovered panics in any goroutine, runtime fatal errors);
// PrevCrashFile is the last run that crashed.
const (
	CrashFile     = "crash.log"
	PrevCrashFile = "crash.prev.log"
)

// CaptureCrashes makes the Go runtime write fatal crash output (the panic
// value and every goroutine's stack) to dir/CrashFile as well as stderr. A
// non-empty crash file left by the previous run is logged and kept as
// dir/PrevCrashFile.
func CaptureCrashes(dir string, log *slog.Logger) error {
	path := filepath.Join(dir, CrashFile)
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		prev := filepath.Join(dir, PrevCrashFile)
		if err := os.Rename(path, prev); err != nil {
			return fmt.Errorf("diag: keep previous crash: %w", err)
		}
		log.Warn("the previous run crashed", "crash_file", prev, "at", fi.ModTime().Format(time.RFC3339), "reason", firstLine(prev))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("diag: crash file: %w", err)
	}
	// SetCrashOutput duplicates the descriptor.
	defer func() { _ = f.Close() }()
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		return fmt.Errorf("diag: crash output: %w", err)
	}
	return nil
}

func firstLine(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text()
	}
	return ""
}

// LogPanic logs a panic unwinding the calling goroutine, with its stack,
// then lets it continue (the process still crashes, and the crash file
// gets the full output). Defer it directly: defer diag.LogPanic(log).
func LogPanic(log *slog.Logger) {
	if r := recover(); r != nil {
		log.Error("panic", "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		panic(r)
	}
}

// Go runs fn on a new goroutine. A panic in fn is recovered and logged at
// error level with name and the stack, so one failed background task does
// not take the app down.
func Go(log *slog.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic in background task", "task", name, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}
