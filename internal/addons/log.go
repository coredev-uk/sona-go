package addons

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// Log limits: a file is rotated at LogBytes and one older copy is kept, so
// an add-on that never stops talking holds at most about twice that on disk.
const (
	LogBytes   = 1 << 20
	logBackups = 1
	// MaxLogLine cuts a longer line, so one add-on can't fill the file with
	// a single write.
	MaxLogLine = 4096
)

// Log is one add-on's log file: what it writes to standard error, and what
// the supervisor has to say of it, as plain lines with the time. The file is
// 0600 and rotated. Log is an io.Writer for the add-on's standard error.
type Log struct {
	f   *logging.RotatingFile
	now func() time.Time

	mu      sync.Mutex
	partial []byte
	tap     func(line string)
}

// OpenLog opens (appending) dir/<id>.log, creating dir with mode 0700.
func OpenLog(dir, id string) (*Log, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("addons: log folder: %w", err)
	}
	f, err := logging.OpenRotatingFile(LogPath(dir, id), LogBytes, logBackups)
	if err != nil {
		return nil, err
	}
	return &Log{f: f, now: time.Now}, nil
}

// LogPath is the add-on's log file in dir.
func LogPath(dir, id string) string { return filepath.Join(dir, id+".log") }

// Write takes the add-on's standard error: complete lines are written with
// the time, control characters (an escape sequence that would repaint a
// terminal) replaced, long lines cut. A partial line waits for its end.
func (l *Log) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.partial = append(l.partial, p...)
	for {
		i := bytes.IndexByte(l.partial, '\n')
		if i < 0 {
			break
		}
		line := l.partial[:i]
		l.partial = l.partial[i+1:]
		if err := l.line("", line); err != nil {
			return len(p), err
		}
	}
	// An endless line must not grow the buffer: flush what is held.
	if len(l.partial) > MaxLogLine {
		err := l.line("", l.partial)
		l.partial = nil
		return len(p), err
	}
	return len(p), nil
}

// Notef writes a line of Sona's own, marked as such: the add-on's lines are
// told apart from the supervisor's.
func (l *Log) Notef(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.line("sona: ", []byte(fmt.Sprintf(format, args...)))
}

func (l *Log) line(prefix string, b []byte) error {
	s := Sanitize(string(b), MaxLogLine)
	line := l.now().UTC().Format("2006-01-02T15:04:05Z") + " " + prefix + s
	if l.tap != nil {
		l.tap(line)
	}
	_, err := io.WriteString(l.f, line+"\n")
	return err
}

// Tap makes fn see every line as it is written to the file, sanitised, with
// its time (`sona addons dev` streams them). fn runs under the log's lock: it
// must not call the log.
func (l *Log) Tap(fn func(line string)) {
	l.mu.Lock()
	l.tap = fn
	l.mu.Unlock()
}

// Close flushes a partial line and closes the file.
func (l *Log) Close() error {
	l.mu.Lock()
	if len(l.partial) > 0 {
		_ = l.line("", l.partial)
		l.partial = nil
	}
	l.mu.Unlock()
	return l.f.Close()
}

// Sanitize is s as one line of plain text for a log or a terminal: control
// characters and bidirectional or other format characters become a space,
// invalid UTF-8 a replacement character, and the result is cut to max bytes
// (at a character) with an ellipsis.
func Sanitize(s string, max int) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			b.WriteRune('\uFFFD')
		case r == '\t' || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.In(r, unicode.Zl, unicode.Zp):
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
		if b.Len() > max {
			break
		}
	}
	out := b.String()
	if len(out) > max {
		out = out[:max]
		for !utf8.ValidString(out) {
			out = out[:len(out)-1]
		}
		out += "…"
	}
	return out
}

// ReadLog returns the last lines of the add-on's log, newest last, at most
// n lines and maxBytes bytes; "" and no error when there is no log yet.
func ReadLog(dir, id string, n, maxBytes int) ([]string, error) {
	f, err := os.Open(LogPath(dir, id))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("addons: read log: %w", err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("addons: read log: %w", err)
	}
	off := max(fi.Size()-int64(maxBytes), 0)
	buf := make([]byte, fi.Size()-off)
	if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
		return nil, fmt.Errorf("addons: read log: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if off > 0 && len(lines) > 0 {
		lines = lines[1:] // cut mid-line
	}
	if len(lines) == 1 && lines[0] == "" {
		return nil, nil
	}
	return lines[max(len(lines)-n, 0):], nil
}
