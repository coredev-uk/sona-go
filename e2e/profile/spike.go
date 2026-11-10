package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// setTheme sets the app's theme ("system" follows the macOS appearance the
// screenshots set).
func setTheme(paths config.Paths, name string) error {
	switch name {
	case config.ThemeLight, config.ThemeDark, config.ThemeSystem:
	default:
		return fmt.Errorf("-theme %q: light, dark or system", name)
	}
	s, err := config.OpenStore(paths.SettingsFile())
	if err != nil {
		return err
	}
	_, err = s.Update(func(s *config.Settings) { s.Theme = name })
	return err
}

// The invented lines the fixture songs "have", timed by line: enough that
// the sheet scrolls and the sung line moves while a song plays.
var lyricLines = []string{
	"Streetlights hum a borrowed tune",
	"while the harbour counts the hours",
	"paper boats on a copper moon",
	"drifting past the sleeping towers",
	"hold the line, hold the line",
	"we were never meant to hurry",
	"every window keeps a light",
	"for the ones who walk in worry",
	"carry me down to the water",
	"carry me out where it's slow",
	"all the clocks can lose their count",
	"all the tides can come and go",
	"hold the line, hold the line",
	"we were never meant to hurry",
	"morning folds the city small",
	"and the gulls begin to bury",
	"every song we left unsung",
	"in the grain of the old pier",
	"we will find the words again",
	"when the evening air is clear",
}

// seedLyrics writes a fresh lyrics_cache row, for each provider that is on
// by default, for every song of the fixture: no provider is asked.
func seedLyrics(ctx context.Context, paths config.Paths) error {
	d, err := db.Open(ctx, paths.DatabaseFile())
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	l := lyrics.Lyrics{Timing: lyrics.TimingLine, Language: "en"}
	for i, ln := range lyricLines {
		begin := time.Duration(i) * 4 * time.Second
		l.Lines = append(l.Lines, lyrics.Line{Begin: begin, End: begin + 4*time.Second, Text: ln})
	}
	now := time.Now()
	for i, a := range fixture.Albums() {
		for j := range a.SongCount() {
			for _, p := range []string{config.LyricsApple, config.LyricsMusixmatch, config.LyricsLRCLIB} {
				l.Provider = p
				body, err := json.Marshal(l)
				if err != nil {
					return err
				}
				if err := d.Q.PutLyricsCache(ctx, dbq.PutLyricsCacheParams{
					Provider: p, Track: "am:" + fixture.SongCatalogID(i, j), Found: 1, Body: body,
					FetchedAt: db.Millis(now), ExpiresAt: db.Millis(now.Add(30 * 24 * time.Hour)),
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
