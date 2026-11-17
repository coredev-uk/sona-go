package sandbox

import (
	"strings"
	"testing"
)

func profileOf(p seatbeltInput) string { return seatbeltProfile(p) }

func TestSeatbeltProfileNetworkOnlyWithThePermission(t *testing.T) {
	in := seatbeltInput{Policy: Policy{Dir: "/a/addon", Data: "/a/data"}}
	if p := profileOf(in); !strings.Contains(p, "(deny network*)") {
		t.Errorf("no network rule:\n%s", p)
	}
	in.Network = true
	if p := profileOf(in); strings.Contains(p, "network") {
		t.Errorf("an add-on with network gets a network rule:\n%s", p)
	}
	in.Network, in.Sockets = false, []string{"/run/user/1/discord-ipc-0"}
	p := profileOf(in)
	deny, allow := strings.Index(p, "(deny network*)"), strings.Index(p, `(literal "/run/user/1/discord-ipc-0")`)
	if deny < 0 || allow < deny {
		t.Errorf("the socket must be allowed after the network is denied:\n%s", p)
	}
}

func TestSeatbeltProfileWritesOnlyToData(t *testing.T) {
	in := seatbeltInput{Policy: Policy{Dir: "/Users/me/addons/x", Data: "/Users/me/state/x/data"}, Hidden: []string{"/Users", "/Users/me"}}
	p := profileOf(in)
	for _, want := range []string{
		"(deny file-write*)",
		`(subpath "/Users/me/state/x/data")`,
		"(deny file-read* file-write*\n  (subpath \"/Users\")\n  (subpath \"/Users/me\")\n)",
		`(subpath "/Users/me/addons/x")`,
	} {
		if !strings.Contains(p, want) {
			t.Errorf("profile lacks %q:\n%s", want, p)
		}
	}
	// Order is meaning: the exceptions follow the rule they are exceptions to.
	hide := strings.Index(p, "(deny file-read* file-write*\n")
	if allow := strings.Index(p[hide:], "(allow file-read*"); allow < 0 {
		t.Errorf("the add-on's folder is not readable after the home is hidden:\n%s", p)
	}
	if allow := strings.Index(p[hide:], "(allow file-write*"); allow < 0 {
		t.Errorf("the data folder is not writable after the home is hidden:\n%s", p)
	}
	if !strings.Contains(p, `(allow file-read-metadata`) || !strings.Contains(p, `(literal "/Users/me")`) {
		t.Errorf("the folders above the add-on's are not reachable:\n%s", p)
	}
}

func TestSeatbeltProfileHasNoTerminalsAndNoKeychain(t *testing.T) {
	p := profileOf(seatbeltInput{Policy: Policy{Dir: "/a", Data: "/b"}})
	for _, want := range []string{"file-ioctl", "/dev/(tty|ttys[0-9]+|pty[a-z0-9]+|console)", `(global-name "com.apple.SecurityServer")`} {
		if !strings.Contains(p, want) {
			t.Errorf("profile lacks %q", want)
		}
	}
}

func TestSeatbeltQuotesPaths(t *testing.T) {
	p := profileOf(seatbeltInput{Policy: Policy{Dir: `/a/"b")(allow default)`, Data: "/c"}})
	if strings.Contains(p, `"b")(allow`) {
		t.Errorf("a path closed its string and added a rule:\n%s", p)
	}
}
