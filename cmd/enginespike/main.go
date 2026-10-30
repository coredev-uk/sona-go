// Command enginespike proves protected Apple Music playback on Linux through
// the Chrome engine: Widevine availability, a public Widevine stream, MusicKit
// configuration with the web player token, and full-track playback.
//
// Usage: enginespike [-mode headless|window] [-song ID | -term "query"] [-env .env]
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback/chrome"
)

func main() {
	mode := flag.String("mode", "headless", "engine mode: headless or window")
	envFile := flag.String("env", ".env", "file providing MUSIC_USER_TOKEN")
	songID := flag.String("song", "", "catalog song ID to play (default: first search hit for -term)")
	term := flag.String("term", "Radiohead Weird Fishes", "search term used when -song is empty")
	seekTo := flag.Float64("seek", 90, "seconds to seek to; past the 30 s preview window proves a full track")
	listen := flag.Duration("listen", 8*time.Second, "how long to observe playback after seeking")
	skipShaka := flag.Bool("skip-shaka", false, "skip the public Widevine stream self-test")
	profile := flag.String("profile", "", "engine profile dir (default $XDG_DATA_HOME/sona/engine)")
	flag.Parse()

	if err := run(*mode, *envFile, *songID, *term, *seekTo, *listen, *skipShaka, *profile); err != nil {
		slog.Error("enginespike failed", "err", err)
		os.Exit(1)
	}
}

func run(mode, envFile, songID, term string, seekTo float64, listen time.Duration, skipShaka bool, profile string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	mut := os.Getenv("MUSIC_USER_TOKEN")
	if mut == "" {
		v, err := readEnv(envFile, "MUSIC_USER_TOKEN")
		if err != nil {
			return err
		}
		mut = v
	}
	if mut == "" {
		return fmt.Errorf("MUSIC_USER_TOKEN not set (env or %s)", envFile)
	}

	// 1. Developer token + MUT validation, entirely in Go.
	dev := devtoken.NewWebPlayer(nil)
	tok, err := dev.Token(ctx)
	if err != nil {
		return err
	}
	step("dev token", tok.String())
	api := applemusic.New(nil, dev, func() string { return mut })
	sf, err := api.Storefront(ctx)
	if err != nil {
		return fmt.Errorf("validate MUT: %w", err)
	}
	step("storefront (MUT valid)", sf.ID)

	var durationMs int64
	if songID == "" {
		songs, err := api.SearchSongs(ctx, sf.ID, term, 1)
		if err != nil {
			return err
		}
		if len(songs) == 0 {
			return fmt.Errorf("no songs for %q", term)
		}
		songID, durationMs = songs[0].ID, songs[0].Attributes.DurationInMillis
		step("song", fmt.Sprintf("%s — %s (%s, %.0fs)", songs[0].Attributes.ArtistName, songs[0].Attributes.Name, songID, float64(durationMs)/1000))
	}

	// 2. Engine.
	if profile == "" {
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		profile = filepath.Join(base, "sona", "engine")
	}
	started := time.Now()
	eng, err := chrome.Start(ctx, chrome.Options{ProfileDir: profile, Mode: chrome.Mode(mode)})
	if err != nil {
		return err
	}
	defer eng.Close()
	step("engine ready", fmt.Sprintf("%s mode in %s", mode, time.Since(started).Round(time.Millisecond)))
	go logEvents(eng.Events())

	var wv json.RawMessage
	if err := eng.Call(ctx, &wv, "widevine"); err != nil {
		return err
	}
	step("widevine", string(wv))

	if !skipShaka {
		var res json.RawMessage
		if err := eng.Call(ctx, &res, "drmSelfTest", 6); err != nil {
			return err
		}
		step("widevine test stream", string(res))
	}

	// 3. MusicKit with the web player token and the user's MUT.
	var conf json.RawMessage
	if err := eng.Call(ctx, &conf, "configure", tok.JWT, mut); err != nil {
		return err
	}
	step("musickit", string(conf))

	var st playState
	if err := eng.Call(ctx, &st, "playSong", songID); err != nil {
		return err
	}
	step("play", st.String())
	time.Sleep(3 * time.Second)
	if err := eng.Call(ctx, &st, "seek", seekTo); err != nil {
		return err
	}
	step("seek", st.String())
	t0 := st.Time
	time.Sleep(listen)
	if err := eng.Call(ctx, &st, "state"); err != nil {
		return err
	}
	step("after listen", st.String())

	full := st.Duration > 31 && st.Time > 31 && st.Time-t0 > listen.Seconds()*0.6
	if !full {
		return fmt.Errorf("not full-track playback: %s", st.String())
	}
	step("RESULT", fmt.Sprintf("full-track protected playback OK (%.1fs advanced past %.0fs of %.0fs)", st.Time-t0, seekTo, st.Duration))
	_ = eng.Call(ctx, nil, "stop")
	return nil
}

type playState struct {
	State      string  `json:"state"`
	Time       float64 `json:"time"`
	Duration   float64 `json:"duration"`
	Authorized bool    `json:"authorized"`
	Item       *struct {
		Title     string `json:"title"`
		IsPreview bool   `json:"isPreview"`
		Flavor    string `json:"flavor"`
	} `json:"item"`
}

func (s playState) String() string {
	item := "-"
	if s.Item != nil {
		item = fmt.Sprintf("%q preview=%v flavor=%s", s.Item.Title, s.Item.IsPreview, s.Item.Flavor)
	}
	return fmt.Sprintf("state=%s t=%.1f/%.1f authorized=%v item=%s", s.State, s.Time, s.Duration, s.Authorized, item)
}

func logEvents(ch <-chan chrome.Event) {
	for ev := range ch {
		if ev.Type == "time" {
			continue
		}
		fmt.Printf("    event %-6s %s\n", ev.Type, truncate(string(ev.Data), 300))
	}
}

func step(name, detail string) { fmt.Printf("==> %-24s %s\n", name, detail) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// readEnv reads key from a KEY=VALUE file without exporting anything else.
func readEnv(path, key string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`), nil
		}
	}
	return "", sc.Err()
}
