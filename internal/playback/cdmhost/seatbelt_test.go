//go:build linux || darwin

package cdmhost

import (
	"errors"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/playback"
)

func TestSeatbeltProfile(t *testing.T) {
	got := seatbeltProfile(seatbeltPaths{
		Hidden:   []string{"/Users/me/Library/Application Support/Sona", "/Users/me/Library/Keychains"},
		Writable: []string{`/Users/me/.cache/gst "odd" \dir`},
		Readable: []string{"/Users/me/Library/Application Support/Sona/widevine/4.10.3112.0/mac_arm64"},
	})
	t.Log("\n" + got)
	lines := strings.Split(got, "\n")
	index := func(s string) int {
		for i, l := range lines {
			if strings.TrimSpace(l) == s {
				return i
			}
		}
		t.Fatalf("profile lacks %q", s)
		return -1
	}
	if lines[0] != "(version 1)" {
		t.Errorf("first line %q", lines[0])
	}
	// Each exception comes after the rule it narrows: later rules win.
	allow, network, noWrite := index("(allow default)"), index("(deny network*)"), index("(deny file-write*)")
	writable := index("(allow file-write*")
	hidden := index("(deny file-read* file-write*")
	readable := index("(allow file-read*")
	if allow >= network || network >= noWrite || noWrite >= writable || writable >= hidden || hidden >= readable {
		t.Errorf("rule order: allow %d, network %d, no write %d, writable %d, hidden %d, readable %d", allow, network, noWrite, writable, hidden, readable)
	}
	for _, want := range []string{
		`(subpath "/private/tmp")`, `(subpath "/private/var/folders")`, `(subpath "/dev")`,
		`(subpath "/Users/me/.cache/gst \"odd\" \\dir")`,
		`(subpath "/Users/me/Library/Application Support/Sona")`,
		`(subpath "/Users/me/Library/Keychains")`,
		`(global-name "com.apple.SecurityServer")`,
		`(subpath "/Users/me/Library/Application Support/Sona/widevine/4.10.3112.0/mac_arm64")`,
		`(literal "/Users/me/Library/Application Support/Sona/widevine/4.10.3112.0")`,
		`(literal "/Users/me/Library/Application Support/Sona")`,
		`(literal "/")`,
	} {
		index(want)
	}
	if strings.Count(got, "(") != strings.Count(got, ")") {
		t.Error("unbalanced parentheses")
	}
	// Nothing hidden: no empty deny rule.
	if strings.Contains(seatbeltProfile(seatbeltPaths{}), "(deny file-read*") {
		t.Error("empty hidden list makes a rule")
	}
}

func TestSeatbeltChoice(t *testing.T) {
	refused := errors.New("exit status 65: sandbox-exec: syntax error")
	for _, c := range []struct {
		name    string
		off     bool
		lookErr error
		probe   error
		gap     playback.SandboxGap
		reason  string
		probed  bool
	}{
		{"works", false, nil, nil, "", "", true},
		{"turned off", true, nil, nil, playback.SandboxGapTurnedOff, "turned off (SONA_SEATBELT=0)", false},
		{"missing", false, errors.New("stat /usr/bin/sandbox-exec: no such file or directory"), nil, playback.SandboxGapNoSeatbelt, "no macOS sandbox: stat /usr/bin/sandbox-exec", false},
		{"refused", false, nil, refused, playback.SandboxGapRefused, "sandbox-exec refused Sona's profile: exit status 65", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			probed := false
			gap, reason := seatbeltChoice(c.off, c.lookErr, func() error { probed = true; return c.probe })
			if gap != c.gap || probed != c.probed || (c.reason == "") != (reason == "") || !strings.Contains(reason, c.reason) {
				t.Errorf("seatbeltChoice = %q %q (probed %v), want %q containing %q (probed %v)", gap, reason, probed, c.gap, c.reason, c.probed)
			}
		})
	}
}

// Outside a sandbox the self-check sees the network reachable: a UDP
// connect to the loopback succeeds (and sends nothing).
func TestInetRefusedOutsideSandbox(t *testing.T) {
	if err := inetRefused(); err == nil || !strings.Contains(err.Error(), "succeeded") {
		t.Fatalf("inetRefused = %v, want the connect to have succeeded", err)
	}
}
