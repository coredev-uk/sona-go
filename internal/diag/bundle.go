package diag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// Defaults for [BundleOptions].
const (
	DefaultLogLines = 400
	// crashTail bounds how much of a crash file the bundle carries; the
	// panic and the crashing goroutine come first in Go's output.
	crashTail = 64 << 10
	// logWindow is how far back from a log file's end lines are read.
	logWindow = 1 << 20
)

// Section is a value the bundle renders as indented JSON under Title.
type Section struct {
	Title string
	Value any
}

// BundleOptions says what goes into a bundle.
type BundleOptions struct {
	Now time.Time
	// Sections are rendered in order (the Diagnostics report, settings).
	// Keys naming secrets or personal data are dropped (see dropKey).
	Sections []Section
	// LogFile is the JSON log; its rotated LogFile.1 supplies older lines
	// when the current file holds fewer than LogLines.
	LogFile string
	// LogLines is how many trailing log lines to include (0: DefaultLogLines).
	LogLines int
	// StateDir holds the crash files (CrashFile, PrevCrashFile).
	StateDir string
	// Home is replaced by "~" everywhere, so paths do not carry the
	// user name.
	Home string
}

// Bundle builds the plain-text diagnostics bundle. Everything in it passes
// through Scrub.
func Bundle(o BundleOptions) string {
	if o.LogLines <= 0 {
		o.LogLines = DefaultLogLines
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sona diagnostics bundle\nGenerated: %s\n", o.Now.UTC().Format(time.RFC3339))
	for _, s := range o.Sections {
		fmt.Fprintf(&b, "\n## %s\n", s.Title)
		b.WriteString(renderJSON(s.Value))
		b.WriteByte('\n')
	}
	for _, name := range []string{PrevCrashFile, CrashFile} {
		path := filepath.Join(o.StateDir, name)
		fi, err := os.Stat(path)
		if err != nil || fi.Size() == 0 {
			continue
		}
		title := "Crash in this run"
		if name == PrevCrashFile {
			title = "Previous crash"
		}
		fmt.Fprintf(&b, "\n## %s (%s, %s)\n", title, name, fi.ModTime().UTC().Format(time.RFC3339))
		b.Write(head(path, crashTail))
		b.WriteByte('\n')
	}
	lines := tailLines(o.LogFile, o.LogLines)
	if len(lines) < o.LogLines {
		lines = append(tailLines(o.LogFile+".1", o.LogLines-len(lines)), lines...)
	}
	fmt.Fprintf(&b, "\n## Log (last %d lines)\n", len(lines))
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return Scrub(b.String(), o.Home)
}

var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// Scrub removes secrets and personal data from text: everything
// logging.Redact removes (JWTs, token headers and keys, bearer
// credentials, MUT-like base64 runs, -ldflags values and linker -X
// assignments, which carry the build's Last.fm and Discord credentials),
// e-mail addresses (Apple IDs), and the home directory (replaced by "~").
// It runs over the log tail as read from disk, so lines written by a build
// whose log handler did not yet redact something are scrubbed here too.
func Scrub(s, home string) string {
	s = logging.Redact(s)
	s = emailRe.ReplaceAllString(s, logging.Redacted)
	if home = strings.TrimRight(home, "/"); home != "" {
		s = strings.ReplaceAll(s, home, "~")
	}
	return s
}

// renderJSON marshals v as indented JSON without the keys dropKey names.
func renderJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("(unavailable: %v)", err)
	}
	var tree any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&tree); err != nil {
		return fmt.Sprintf("(unavailable: %v)", err)
	}
	out, err := json.MarshalIndent(prune(tree), "", "  ")
	if err != nil {
		return fmt.Sprintf("(unavailable: %v)", err)
	}
	return string(out)
}

func prune(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, sub := range t {
			if dropKey(k) {
				delete(t, k)
				continue
			}
			t[k] = prune(sub)
		}
	case []any:
		for i, sub := range t {
			t[i] = prune(sub)
		}
	}
	return v
}

// dropKey reports whether a JSON key names a secret or personal data:
// credentials, account identity, and what the user was looking at.
func dropKey(key string) bool {
	k := strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
	switch k {
	case "mut", "authorization", "cookie", "password", "secret", "username", "email",
		"appleid", "account", "dsid", "lastroute":
		return true
	}
	return strings.HasSuffix(k, "token")
}

// head reads up to n bytes from the start of path.
func head(path string, n int64) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, _ := io.ReadAll(io.LimitReader(f, n))
	return bytes.TrimRight(data, "\n")
}

// tailLines returns up to n complete lines from the end of path.
func tailLines(path string, n int) []string {
	if n <= 0 || path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil
	}
	off := max(fi.Size()-logWindow, 0)
	data, err := io.ReadAll(io.NewSectionReader(f, off, fi.Size()-off))
	if err != nil {
		return nil
	}
	if off > 0 {
		// Drop the partial first line.
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines[max(len(lines)-n, 0):]
}
