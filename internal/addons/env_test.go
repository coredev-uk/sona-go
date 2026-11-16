package addons

import (
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
	got := Environment(parent, "demo")
	for _, want := range []string{"HOME=/home/u", "PATH=/usr/bin", "LANG=en_GB.UTF-8", "LC_ALL=C", "TZ=UTC", "SONA_ADDON_ID=demo", "SONA_ADDON_PROTOCOL=2"} {
		if !slices.Contains(got, want) {
			t.Errorf("environment lacks %q: %v", want, got)
		}
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
	got := environment([]string{"SystemRoot=C:\\Windows", "Path=C:\\x", "MUSIC_USER_TOKEN=t", "Temp=C:\\t"}, "demo", true)
	if !slices.Contains(got, "SystemRoot=C:\\Windows") || !slices.Contains(got, "Path=C:\\x") || !slices.Contains(got, "Temp=C:\\t") {
		t.Errorf("environment = %v", got)
	}
	for _, kv := range got {
		if strings.HasPrefix(kv, "MUSIC_USER_TOKEN") {
			t.Errorf("token in %v", got)
		}
	}
}
