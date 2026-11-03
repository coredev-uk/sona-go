package services

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

func snapAt(state playback.State, uid uint64, title string) coordinator.Snapshot {
	s := coordinator.Snapshot{State: state, Index: -1}
	if uid != 0 {
		s.Current = &coordinator.Entry{UID: uid, Item: coordinator.Item{ID: title, Title: title, Duration: 3 * time.Minute}}
		s.Index = 0
	}
	return s
}

func TestIntegrationsPlan(t *testing.T) {
	on := config.Integrations{Notifications: true, InhibitSleep: true}
	type want struct {
		announce string // title, "" for none
		inhibit  string // "take", "drop" or ""
	}
	steps := []struct {
		name string
		snap coordinator.Snapshot
		set  config.Integrations
		want want
	}{
		{"restored paused", snapAt(playback.StatePaused, 1, "A"), on, want{}},
		{"play restored", snapAt(playback.StatePlaying, 1, "A"), on, want{"A", "take"}},
		{"pause", snapAt(playback.StatePaused, 1, "A"), on, want{"", "drop"}},
		{"resume same slot", snapAt(playback.StatePlaying, 1, "A"), on, want{"", "take"}},
		{"next loads", snapAt(playback.StateLoading, 2, "B"), on, want{}},
		{"next plays", snapAt(playback.StatePlaying, 2, "B"), on, want{"B", ""}},
		{"settings off", snapAt(playback.StatePlaying, 2, "B"), config.Integrations{}, want{"", "drop"}},
		{"track while off", snapAt(playback.StatePlaying, 3, "C"), config.Integrations{}, want{}},
		{"on again mid-track", snapAt(playback.StatePlaying, 3, "C"), on, want{"", "take"}},
		{"queue ends", snapAt(playback.StateEnded, 3, "C"), on, want{"", "drop"}},
		{"stopped", snapAt(playback.StateIdle, 0, ""), on, want{}},
		{"same song again in a new slot", snapAt(playback.StatePlaying, 4, "C"), on, want{"C", "take"}},
		{"failed", snapAt(playback.StateError, 4, "C"), on, want{"", "drop"}},
	}
	var st integrationsState
	for _, s := range steps {
		p := st.plan(s.snap, s.set, nil, time.Time{})
		got := want{}
		if p.announce != nil {
			got.announce = p.announce.Title
		}
		if p.inhibit != nil {
			got.inhibit = map[bool]string{true: "take", false: "drop"}[*p.inhibit]
		}
		if got != s.want {
			t.Errorf("%s: got %+v, want %+v", s.name, got, s.want)
		}
	}
}

func TestIntegrationsPlanPresence(t *testing.T) {
	var st integrationsState
	set := config.Integrations{DiscordPresence: true}
	plan := func(snap coordinator.Snapshot, set config.Integrations) integrationsPlan {
		return st.plan(snap, set, nil, time.Time{})
	}
	if p := plan(snapAt(playback.StatePlaying, 1, "A"), set); p.presence == nil || p.presence.act == nil || p.presence.act.Title != "A" {
		t.Fatalf("playing: presence = %+v", p.presence)
	}
	if p := plan(snapAt(playback.StateLoading, 2, "B"), set); p.presence != nil {
		t.Fatalf("loading changed the presence: %+v", p.presence)
	}
	if p := plan(snapAt(playback.StatePaused, 2, "B"), set); p.presence == nil || p.presence.act != nil || !p.presence.enabled {
		t.Fatalf("paused: presence = %+v, want cleared", p.presence)
	}
	if p := plan(snapAt(playback.StatePlaying, 2, "B"), config.Integrations{}); p.presence == nil || p.presence.act != nil || p.presence.enabled {
		t.Fatalf("disabled: presence = %+v, want cleared and disabled", p.presence)
	}
}

// A private listen shows no Discord status, from the toggle on until the
// listen ends, even if private mode is turned off meanwhile.
func TestIntegrationsPlanPresencePrivate(t *testing.T) {
	now := time.Unix(1000, 0)
	m := privatemode.New(privatemode.Options{Now: func() time.Time { return now }})
	var st integrationsState
	set := config.Integrations{DiscordPresence: true}
	shown := func(uid uint64, title string) bool {
		t.Helper()
		p := st.plan(snapAt(playback.StatePlaying, uid, title), set, m.Since, now)
		if p.presence == nil || !p.presence.enabled {
			t.Fatalf("%s: presence = %+v", title, p.presence)
		}
		return p.presence.act != nil
	}
	if !shown(1, "A") {
		t.Fatal("public listen: no presence")
	}
	now = now.Add(time.Minute)
	if _, err := m.Set(true); err != nil {
		t.Fatal(err)
	}
	if shown(1, "A") {
		t.Fatal("private mode turned on mid-listen: presence still shown")
	}
	now = now.Add(time.Minute)
	if _, err := m.Set(false); err != nil {
		t.Fatal(err)
	}
	if shown(1, "A") {
		t.Fatal("private mode turned off mid-listen: the private listen went public")
	}
	now = now.Add(time.Minute)
	if !shown(2, "B") {
		t.Fatal("next listen after private mode turned off: no presence")
	}
}

func TestPresenceFor(t *testing.T) {
	at := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	snap := snapAt(playback.StatePlaying, 1, "Come Together")
	snap.Current.Item.Artist, snap.Current.Item.Album = "The Beatles", "Abbey Road"
	snap.Current.Item.Artwork = "https://is1-ssl.mzstatic.com/image/thumb/Music/x/{w}x{h}{c}.{f}"
	snap.At, snap.Position, snap.Duration = at, 90*time.Second, 259*time.Second

	a := presenceFor(snap)
	if a == nil {
		t.Fatal("no presence while playing")
	}
	if !a.Start.Equal(at.Add(-90*time.Second)) || !a.End.Equal(at.Add(169*time.Second)) {
		t.Errorf("start %v end %v", a.Start, a.End)
	}
	if a.ArtworkURL != "https://is1-ssl.mzstatic.com/image/thumb/Music/x/512x512bb.jpg" {
		t.Errorf("artwork = %q", a.ArtworkURL)
	}

	snap.Duration = 0 // engine has no duration yet: the catalog's
	if a := presenceFor(snap); !a.End.Equal(a.Start.Add(3 * time.Minute)) {
		t.Errorf("end without engine duration = %v", a.End)
	}
	snap.State = playback.StatePaused
	if presenceFor(snap) != nil {
		t.Error("presence while paused")
	}
}

func TestPublicArtURL(t *testing.T) {
	good := map[string]string{
		"https://is1-ssl.mzstatic.com/image/thumb/x/{w}x{h}bb.jpg":    "https://is1-ssl.mzstatic.com/image/thumb/x/512x512bb.jpg",
		" https://IS5-SSL.mzstatic.com/image/thumb/x/{w}x{h}{c}.{f} ": "https://IS5-SSL.mzstatic.com/image/thumb/x/512x512bb.jpg",
	}
	for in, want := range good {
		if got := publicArtURL(in, 512); got != want {
			t.Errorf("publicArtURL(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{
		"",
		"/art/abc/512.jpg",
		"http://is1-ssl.mzstatic.com/a/{w}x{h}bb.jpg",
		"https://evil.example/a/{w}x{h}bb.jpg",
		"https://mzstatic.com.evil.example/a/{w}x{h}bb.jpg",
		"https://u:p@is1-ssl.mzstatic.com/a/{w}x{h}bb.jpg",
		"https://is1-ssl.mzstatic.com:8443/a/{w}x{h}bb.jpg",
		"https://is1-ssl.mzstatic.com/a/{w}x{h}bb.jpg?token=x",
	} {
		if got := publicArtURL(bad, 512); got != "" {
			t.Errorf("publicArtURL(%q) = %q, want rejected", bad, got)
		}
	}
}

// The notification offers Next after a song, not after a live station.
func TestTrackNotificationActions(t *testing.T) {
	keys := func(item coordinator.Item) []string {
		var out []string
		for _, a := range trackNotification(l10n.NewPrinter("en", nil), item, "").Actions {
			out = append(out, a.Key)
		}
		return out
	}
	if got := keys(coordinator.Item{ID: "1440", Title: "Song", Duration: time.Minute}); !slices.Equal(got, []string{notify.DefaultAction, actionNext}) {
		t.Errorf("song actions = %v", got)
	}
	if got := keys(coordinator.Item{ID: "ra.1", Title: "KISS"}); !slices.Equal(got, []string{notify.DefaultAction}) {
		t.Errorf("live station actions = %v", got)
	}
}
