package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Escape sequences a hostile title, lyric or error could carry: a window
// title (OSC 0), an OSC 52 clipboard write, a CSI that clears the screen,
// an 8-bit CSI, a bell, a NUL and a bidirectional override.
const (
	evilOSC52 = "\x1b]52;c;cGF5bG9hZA==\x07"
	evilTitle = "\x1b]0;pwned\x07"
	evilCSI   = "\x1b[2J\x1b[H"
	evilC1    = "\u009b2J\u009d0;x\u009c"
	evilMisc  = "\x07\x00\u202e"
)

func evil(s string) string { return evilOSC52 + evilTitle + s + evilCSI + evilC1 + evilMisc }

// noControl fails the test when s still holds anything a terminal would
// act on.
func noControl(t *testing.T, what, s string) {
	t.Helper()
	for _, bad := range []string{"\x1b", "\x07", "\x00", "\u009b", "\u009d", "\u009c", "\u202e"} {
		if strings.Contains(s, bad) {
			t.Errorf("%s still holds %q: %q", what, bad, s)
		}
	}
}

func TestTextOutputHasNoTerminalControls(t *testing.T) {
	bad := &Track{ID: "1", Title: evil("Come Together"), Artist: evil("The Beatles"), Album: evil("Abbey Road"), Duration: 259}
	lyrics := Lyrics{
		Track: *bad, State: StatePlaying, Provider: "lrclib", Timing: "line", Synced: true,
		Lines: []LyricLine{
			{Start: 1, End: 2, Text: evil("Here comes the sun"), Translation: evil("Ahí viene el sol")},
			{Start: 3, End: 4, Text: evilCSI, Background: evil("(oh)")},
		},
	}
	tr := &fakeTransport{
		results: map[string]any{
			"status":  Status{Running: true, State: StatePlaying, Track: bad, Repeat: "off", Error: evil("failed")},
			"lyrics":  lyrics,
			"queue":   Queue{Tracks: []QueueTrack{{Track: *bad}}},
			"search":  SearchResults{Term: "x", Results: []SearchResult{{Kind: evil("song"), Name: evil("Name"), Subtitle: evil("Sub"), URL: evil("https://example.com")}}},
			"like":    Like{Track: *bad, Liked: true},
			"library": LibraryItems{Kind: "album", Total: 1, Items: []LibraryItem{{Name: evil("Album"), Subtitle: evil("Artist")}}},
			"open":    Opened{Kind: evil("album"), ID: evil("1")},
		},
		statuses: []Status{{Running: true, State: StatePlaying, Track: bad, Repeat: "off"}},
		errs:     map[string]error{"next": &Error{Code: CodeFailed, Message: evil("it broke")}},
	}
	for _, argv := range [][]string{
		{"status"},
		{"status", "--format", "{{.track.title}} / {{.track.artist}} / {{.error}}"},
		{"status", "--follow"},
		{"status", "--follow", "--format", "{{.track.title}}"},
		{"lyrics"},
		{"lyrics", "--lrc"},
		{"lyrics", "--translation"},
		{"queue"},
		{"search", "x"},
		{"like"},
		{"library", "albums"},
		{"open", "https://music.apple.com/gb/album/x/1"},
	} {
		code, out, errOut := run(t, tr, argv...)
		if code != ExitOK && errOut == "" {
			t.Errorf("%q: exit %d with no message", argv, code)
		}
		noControl(t, strings.Join(argv, " ")+" stdout", out)
		noControl(t, strings.Join(argv, " ")+" stderr", errOut)
		if len(out) == 0 && len(errOut) == 0 {
			t.Errorf("%q printed nothing", argv)
		}
	}

	// What is readable stays.
	_, out, _ := run(t, tr, "status")
	for _, want := range []string{"Come Together", "The Beatles", "Abbey Road", "failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("status lost %q: %q", want, out)
		}
	}
	clean := &fakeTransport{results: map[string]any{"status": Status{Running: true, State: StatePlaying, Track: song, Repeat: "off", Error: "failed"}}}
	_, want, _ := run(t, clean, "status")
	if strings.Count(out, "\n") != strings.Count(want, "\n") {
		t.Errorf("status has %d lines, want %d (a line break in a title must not add one): %q", strings.Count(out, "\n"), strings.Count(want, "\n"), out)
	}

	// An error message from the service is text from outside too.
	code, out, errOut := run(t, tr, "next")
	if code != ExitFailed || out != "" || !strings.Contains(errOut, "it broke") || strings.Count(errOut, "\n") != 1 {
		t.Errorf("next: %d, %q, %q", code, out, errOut)
	}
	noControl(t, "error", errOut)
}

func TestFollowedLyricsLineHasNoTerminalControls(t *testing.T) {
	for _, argv := range [][]string{
		{"lyrics", "--follow"},
		{"lyrics", "--follow", "--format", "{{.text}}|{{.track.title}}|{{.line.text}}"},
	} {
		in, err := parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		line := LyricLine{Start: 1, End: 2, Text: evil("sung")}
		ev := LyricsNow{
			Running: true, State: StatePlaying, Lyrics: lyricsReady, Synced: true, Index: 0,
			Track: &Track{ID: "1", Title: evil("Title")}, Line: &line, Text: line.shown(showOriginal),
		}
		var b bytes.Buffer
		if err := writeLyricsNow(&b, in, ev); err != nil {
			t.Fatal(err)
		}
		noControl(t, strings.Join(argv, " "), b.String())
		if !strings.Contains(b.String(), "sung") {
			t.Errorf("%q lost the line: %q", argv, b.String())
		}
	}
}

// --json is the data as received: its control characters are escaped by
// JSON, and decoding them gives back what the service sent.
func TestJSONOutputIsNotChanged(t *testing.T) {
	bad := &Track{ID: "1", Title: evil("Come Together")}
	tr := &fakeTransport{
		results:  map[string]any{"status": Status{Running: true, State: StatePlaying, Track: bad, Repeat: "off"}},
		statuses: []Status{{Running: true, State: StatePlaying, Track: bad, Repeat: "off"}},
	}
	for _, argv := range [][]string{{"status", "--json"}, {"status", "--follow", "--json"}} {
		code, out, _ := run(t, tr, argv...)
		var s Status
		first, _, _ := strings.Cut(out, "\n")
		if err := json.Unmarshal([]byte(first), &s); err != nil || code != ExitOK || s.Track == nil || s.Track.Title != bad.Title {
			t.Errorf("%q: %d, %v, %q", argv, code, err, out)
		}
	}
}

func TestPlainCopiesAndLeavesTheOriginal(t *testing.T) {
	orig := Status{Running: true, Track: &Track{Title: evil("a")}, Error: evil("e")}
	got := plain(orig)
	if got.Track == orig.Track || strings.TrimSpace(got.Track.Title) != "a" || strings.TrimSpace(got.Error) != "e" {
		t.Errorf("plain = %+v", got)
	}
	if orig.Track.Title != evil("a") || !strings.Contains(orig.Error, "\x1b") {
		t.Errorf("plain changed its argument: %+v", orig)
	}
	noControl(t, "plain", got.Track.Title+got.Error)
}
