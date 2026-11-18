package addons

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEnvironmentIsAnAllowlist(t *testing.T) {
	parent := []string{
		"HOME=/home/u", "PATH=/usr/bin", "LANG=en_GB.UTF-8", "LC_ALL=C", "TZ=UTC",
		"MUSIC_USER_TOKEN=secret", "SONA_LASTFM_KEY=k", "SONA_UPDATE_KEY=u", "SONA_PRIVATE=0",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus", "MYGO_AUTOMATION=1",
		"SONA_ADDON_ID=other", "XDG_RUNTIME_DIR=/run/user/1000", "SSH_AUTH_SOCK=/x", "malformed",
	}
	got := Environment(parent, "demo", "/state/addons/demo/data")
	for _, want := range []string{
		"PATH=/usr/bin", "LANG=en_GB.UTF-8", "LC_ALL=C", "TZ=UTC", "SONA_ADDON_ID=demo", "SONA_ADDON_PROTOCOL=2",
		// The user's home is not the add-on's: its own is its data folder.
		"SONA_ADDON_DATA=/state/addons/demo/data", "HOME=/state/addons/demo/data", "TMPDIR=/state/addons/demo/data/tmp",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("environment lacks %q: %v", want, got)
		}
	}
	if slices.Contains(got, "HOME=/home/u") {
		t.Errorf("the user's home reached the add-on: %v", got)
	}
	for _, kv := range got {
		for _, banned := range []string{"MUSIC_USER_TOKEN", "SONA_LASTFM_KEY", "SONA_UPDATE_KEY", "SONA_PRIVATE", "DBUS_", "MYGO_AUTOMATION", "XDG_RUNTIME_DIR", "SSH_AUTH_SOCK", "malformed"} {
			if strings.HasPrefix(kv, banned) {
				t.Errorf("environment has %q", kv)
			}
		}
	}
	if n := slices.IndexFunc(got, func(kv string) bool { return strings.HasPrefix(kv, "SONA_ADDON_ID=") }); got[n] != "SONA_ADDON_ID=demo" || slices.Contains(got, "SONA_ADDON_ID=other") {
		t.Errorf("the parent's SONA_ADDON_ID got through: %v", got)
	}
}

func TestWindowsEnvironmentKeepsWhatAProcessNeeds(t *testing.T) {
	got := environment([]string{"SystemRoot=C:\\Windows", "Path=C:\\x", "MUSIC_USER_TOKEN=t", "Temp=C:\\t", "UserProfile=C:\\Users\\u"}, "demo", `C:\data`, true)
	for _, want := range []string{"SystemRoot=C:\\Windows", "Path=C:\\x", `USERPROFILE=C:\data`, "TEMP=" + filepath.Join(`C:\data`, TempName), "TMP=" + filepath.Join(`C:\data`, TempName), `SONA_ADDON_DATA=C:\data`} {
		if !slices.Contains(got, want) {
			t.Errorf("environment lacks %q: %v", want, got)
		}
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "Temp=") || strings.HasPrefix(kv, "UserProfile=") {
			t.Errorf("the user's own %s reached the add-on", kv)
		}
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "MUSIC_USER_TOKEN") {
			t.Errorf("token in %v", got)
		}
	}
}

// Sona's own add-ons also inherit the proxy and certificate settings their
// HTTP client reads, as theirs did inside Sona; an add-on of the user's does
// not, and no secret of Sona's reaches either.
func TestEnvironmentGivesBundledAddonsTheirProxy(t *testing.T) {
	parent := []string{"HTTPS_PROXY=http://proxy:3128", "no_proxy=localhost", "SSL_CERT_FILE=/etc/ssl/cert.pem", "SONA_LASTFM_SECRET=s", "MUSIC_USER_TOKEN=t"}
	own := Environment(parent, "listenbrainz", "/data")
	for _, want := range parent[:3] {
		if !slices.Contains(own, want) {
			t.Errorf("a bundled add-on lacks %q: %v", want, own)
		}
	}
	for _, kv := range append(Environment(parent, "listenbrainz", "/data"), Environment(parent, "demo", "/data")...) {
		if strings.HasPrefix(kv, "SONA_LASTFM") || strings.HasPrefix(kv, "MUSIC_USER_TOKEN") {
			t.Errorf("a secret reached an add-on: %q", kv)
		}
	}
	for _, kv := range Environment(parent, "demo", "/data") {
		if strings.Contains(kv, "PROXY") || strings.Contains(kv, "proxy") || strings.HasPrefix(kv, "SSL_") {
			t.Errorf("an add-on of the user's got %q", kv)
		}
	}
}
