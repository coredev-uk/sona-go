package addons

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLogLinesAreStampedSanitisedAndCut(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLog(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = l.Write([]byte("first\nsecond \x1b[31mred\x1b[0m\npart"))
	_, _ = l.Write([]byte("ial\n" + strings.Repeat("x", 3*MaxLogLine) + "\n"))
	l.Notef("exited with %d", 1)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	lines, err := ReadLog(dir, "demo", 10, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 {
		t.Fatalf("lines = %q", lines)
	}
	for _, s := range lines {
		if len(s) < 21 || s[4] != '-' || s[19] != 'Z' {
			t.Errorf("no time on %q", s)
		}
		if strings.ContainsRune(s, 0x1b) {
			t.Errorf("escape in %q", s)
		}
	}
	if !strings.HasSuffix(lines[0], "Z first") || !strings.HasSuffix(lines[2], "Z partial") || !strings.HasSuffix(lines[4], "Z sona: exited with 1") {
		t.Errorf("lines = %q", lines)
	}
	if len(lines[3]) > 21+MaxLogLine+len("…") {
		t.Errorf("long line not cut: %d", len(lines[3]))
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(LogPath(dir, "demo"))
		if err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf("log mode = %v, %v", fi.Mode(), err)
		}
	}
}

func TestLogRotates(t *testing.T) {
	dir := t.TempDir()
	l, err := OpenLog(dir, "chatty")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("y", 1000) + "\n"
	for range 3 * LogBytes / 1000 {
		_, _ = l.Write([]byte(line))
	}
	_ = l.Close()
	var total int64
	for _, name := range []string{"chatty.log", "chatty.log.1"} {
		fi, err := os.Stat(dir + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Size() > LogBytes+2048 {
			t.Errorf("%s is %d bytes", name, fi.Size())
		}
		total += fi.Size()
	}
	if _, err := os.Stat(dir + "/chatty.log.2"); err == nil {
		t.Error("kept more than one older copy")
	}
	if total > 2*LogBytes+4096 {
		t.Errorf("logs hold %d bytes", total)
	}
}

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"plain":            "plain",
		"a\x1b[2Jb":        "a [2Jb",
		"tab\there":        "tab here",
		"bidi\u202egpj":    "bidi gpj",
		"bad\xffbyte":      "bad\uFFFDbyte",
		"cr\rline\x00null": "cr line null",
	} {
		if got := Sanitize(in, 100); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Sanitize("héllo wörld", 6); got != "héllo…" {
		t.Errorf("cut = %q", got)
	}
}
