//go:build linux || (darwin && cgo)

// Command playspike proves Sona's native playback path end to end: the
// Widevine CDM host child decrypts a full Apple Music track and plays it
// through GStreamer (PipeWire on Linux, Core Audio on macOS). It reports
// play→first-audio latency, position checkpoints past the 30 s preview
// window, the child's audio stream (Linux), the gap between two
// consecutive tracks, and parent+child memory. Every licence exchange is
// printed (HTTP status, Apple's status and message, the licence's size;
// never the token or the licence), and the default and -activity modes end
// with one line saying whether Apple licensed this host:
//
//	licence: granted
//	licence: refused (<why>)
//	licence: not requested (<why>)
//
// Usage: playspike [-env .env] [-song ID | -term "query"] [-seek 90] [-next-term "query"] [-crash] [-quality 256|64] [-cdm-dir DIR]
//
// The CDM is the one Sona downloads (installed under Sona's data directory,
// or -cdm-dir, on first use).
//
//	playspike -preview [-storefront gb]        30-second previews: developer token only (no MUSIC_USER_TOKEN, .env or CDM)
//
//	playspike -activity [-private=false]       play activity, private by default (Recently Played must stay unchanged)
//
//	playspike -soak 3h [-soak-items ID,ID,…]   long session: licence renewals and stalls
//	playspike -probe-lossless SONG_ID          Widevine licences for every enhancedHls variant
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/native"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

func main() {
	cdmhost.ChildMain()

	envFile := flag.String("env", ".env", "file providing MUSIC_USER_TOKEN when it is not in the environment")
	songID := flag.String("song", "", "song ID: catalog, or a library upload (i.…) (default: first search hit for -term)")
	term := flag.String("term", "Radiohead Weird Fishes", "search term used when -song is empty")
	seekTo := flag.Float64("seek", 90, "seconds to seek to; past the 30 s preview window proves a full track")
	listen := flag.Duration("listen", 12*time.Second, "how long to observe playback after seeking")
	nextID := flag.String("next", "", "second song for the gap test (default: first hit for -next-term)")
	nextTerm := flag.String("next-term", "Radiohead Reckoner", "search term for the second song")
	gap := flag.Bool("gap", true, "play into the next track to measure the gap")
	crash := flag.Bool("crash", false, "kill the host mid-track and measure recovery")
	cdmDir := flag.String("cdm-dir", "", "install directory of the downloaded Widevine CDM (default: Sona's, <data>/widevine under the current XDG directories)")
	activity := flag.Bool("activity", false, "play -song (or -term's hit) then -next (or -next-term's) through the coordinator with Apple play-activity reporting, and check Recently Played")
	container := flag.String("container", "", "with -activity: kind:id (album, playlist or station) the songs are queued from, reported with the plays and looked for in Recently Played")
	private := flag.Bool("private", true, "with -activity: report the plays as private (\"private-enabled\": true; kept out of the listening history) and check Recently Played stays unchanged; -private=false reports them to the account and checks they show up. SONA_PRIVATE=1 forces private")
	quality := flag.Int("quality", 256, "AAC bitrate to play: 256 or 64 (kbps)")
	soak := flag.Duration("soak", 0, "play -soak-items (or the first 25 hits for -term) back to back for this long, logging licence renewals and stalls")
	soakItems := flag.String("soak-items", "", "comma-separated song or station (ra.…) IDs for -soak")
	probeLossless := flag.String("probe-lossless", "", "catalog song ID: request a Widevine licence for every key of its enhancedHls (lossless/Atmos) master and print the results")
	verbose := flag.Bool("v", false, "debug logging")
	preview := flag.Bool("preview", false, "play -song (or -term's hit), then -next (or -next-term's), as 30-second previews with the developer token only: no MUSIC_USER_TOKEN, .env, CDM or licence; checks that none is requested")
	storefront := flag.String("storefront", "us", "with -preview: the catalog storefront songs are looked up in")
	flag.StringVar(&outputDevice, "device", "", deviceHelp)
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	var err error
	q := applemusic.Quality(*quality)
	if q != applemusic.Quality256 && q != applemusic.Quality64 {
		log.Error("playspike: -quality must be 256 or 64", "quality", *quality)
		os.Exit(2)
	}
	// The default, -preview and -activity modes end with the licence verdict.
	verdict := *probeLossless == "" && *soak <= 0
	if *preview {
		// No CDM: a computer that never downloaded one plays previews.
		err = runPreview(log, *storefront, *songID, *term, *nextID, *nextTerm, *listen)
		if err != nil {
			log.Error("playspike failed", "err", err)
		}
		fmt.Println(licences.verdict(err))
		if err != nil {
			os.Exit(1)
		}
		return
	}
	lib, err := cdmLibrary(log, *cdmDir)
	if err != nil {
		log.Error("playspike: Widevine CDM", "code", cdm.CodeOf(err), "err", err)
		if verdict {
			fmt.Println(licences.verdict(fmt.Errorf("widevine CDM: %w", err)))
		}
		os.Exit(1)
	}
	switch {
	case *probeLossless != "":
		err = runProbeLossless(log, *envFile, *probeLossless, lib)
	case *soak > 0:
		var items []string
		if *soakItems != "" {
			items = strings.Split(*soakItems, ",")
		}
		err = runSoak(log, *envFile, items, *term, *soak, lib, q)
	case *activity:
		// SONA_PRIVATE=1 (and any value privatemode.Env does not read as
		// off) wins over -private=false.
		if o, err := privatemode.Env(os.Getenv(privatemode.EnvVar)); o == privatemode.OverrideOn {
			if err != nil {
				log.Warn("playspike", "err", err)
			}
			*private = true
		}
		err = runActivity(log, *envFile, *songID, *term, *nextID, *nextTerm, *listen, lib, *container, *private)
	default:
		err = run(log, *envFile, *songID, *term, *seekTo, *listen, *nextID, *nextTerm, *gap, *crash, lib, q)
	}
	if err != nil {
		log.Error("playspike failed", "err", err)
	}
	if verdict {
		fmt.Println(licences.verdict(err))
	}
	if err != nil {
		os.Exit(1)
	}
}

// licences watches every licence exchange of the Apple client (signIn).
var licences = newLicenceTap()

func step(name string, kv ...any) {
	fmt.Printf("%-28s", name)
	for i := 0; i+1 < len(kv); i += 2 {
		fmt.Printf(" %v=%v", kv[i], kv[i+1])
	}
	fmt.Println()
}

// recorder keeps engine events with their arrival time.
type recorder struct {
	events chan stamped
}

type stamped struct {
	at time.Time
	ev playback.Event
}

func (r *recorder) waitFor(ctx context.Context, ok func(playback.Event) bool) (stamped, error) {
	for {
		select {
		case s, open := <-r.events:
			if !open {
				return stamped{}, fmt.Errorf("engine closed")
			}
			if ok(s.ev) {
				return s, nil
			}
		case <-ctx.Done():
			return stamped{}, ctx.Err()
		}
	}
}

func (r *recorder) drain() {
	for {
		select {
		case <-r.events:
		default:
			return
		}
	}
}

func run(log *slog.Logger, envFile, songID, term string, seekTo float64, listen time.Duration, nextID, nextTerm string, gap, crash bool, lib string, quality applemusic.Quality) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	api, sf, err := signIn(ctx, envFile)
	if err != nil {
		return err
	}

	song, err := resolve(ctx, api, sf.ID, songID, term)
	if err != nil {
		return err
	}
	step("song", "id", song.ID, "name", fmt.Sprintf("%q", song.Attributes.Name), "catalog_ms", song.Attributes.DurationInMillis)

	eng := native.New(native.Options{Client: api, CDM: native.CDMPath(lib), Logger: log, IdleTimeout: -1, Quality: quality})
	defer eng.Close()
	if err := eng.SetOutputDevice(ctx, outputDevice); err != nil {
		return err
	}
	rec := &recorder{events: make(chan stamped, 256)}
	go func() {
		for ev := range eng.Events() {
			rec.events <- stamped{time.Now(), ev}
		}
		close(rec.events)
	}()

	// 1. Cold play: host spawn + webPlayback + licence + first segment.
	t0 := time.Now()
	if err := eng.Play(ctx, song.ID, 0); err != nil {
		return fmt.Errorf("play: %w", err)
	}
	coldLatency := time.Since(t0)
	info, _ := eng.Host()
	st := eng.Status()
	step("play → first audio (cold)", "latency", coldLatency.Round(time.Millisecond), "state", st.State, "duration", st.Duration.Round(time.Millisecond))
	step("host", "pid", info.PID, "cdm", info.CDMVersion, "cdm_interface", info.CDMInterface, "host_interface", info.HostInterface)
	step("audio", "gstreamer", info.GStreamer, "decoder", info.Decoder, "sink", info.Sink)
	step("cdm path", "path", info.CDMPath)
	step("sandbox", append([]any{"kind", info.Sandbox.Kind()}, info.Sandbox.Attrs()...)...)

	time.Sleep(2 * time.Second)
	audioStream(info.PID)

	// 2. Seek past the preview window and watch the position advance.
	target := time.Duration(seekTo * float64(time.Second))
	t1 := time.Now()
	if err := eng.Seek(ctx, target); err != nil {
		return fmt.Errorf("seek: %w", err)
	}
	if _, err := rec.waitFor(ctx, func(ev playback.Event) bool {
		return ev.Kind == playback.EventState && ev.Status.State == playback.StatePlaying && ev.Status.Position >= target && ev.Status.UpdatedAt.After(t1)
	}); err != nil {
		return fmt.Errorf("seek: no audio: %w", err)
	}
	step("seek → first audio", "target", target, "latency", time.Since(t1).Round(time.Millisecond))
	deadline := time.Now().Add(listen)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		s := eng.Status()
		pos := s.Position
		if s.State == playback.StatePlaying {
			pos += time.Since(s.UpdatedAt)
		}
		step("checkpoint", "state", s.State, "position", pos.Round(100*time.Millisecond))
	}
	s := eng.Status()
	final := s.Position + time.Since(s.UpdatedAt)
	need := target + 10*time.Second
	if s.Duration > 0 && s.Duration < need { // a short track (an upload clip) plays to its end
		if s.State != playback.StateEnded {
			return fmt.Errorf("short track: state %s at %v, want ended (duration %v)", s.State, final, s.Duration)
		}
		step("played to the end", "duration", s.Duration.Round(10*time.Millisecond))
	} else {
		if s.State != playback.StatePlaying || final < need {
			return fmt.Errorf("full-track proof failed: state %s at %v (need > %v)", s.State, final, need)
		}
		step("full track (preview=false)", "advanced_past_target", (final - target).Round(100*time.Millisecond))
	}
	memory("memory while playing", info.PID)

	// 3. Crash recovery: kill the host, expect a transparent resume.
	if crash {
		before := eng.Status()
		pos := before.Position + time.Since(before.UpdatedAt)
		rec.drain()
		tk := time.Now()
		if err := syscall.Kill(info.PID, syscall.SIGKILL); err != nil {
			return fmt.Errorf("kill host: %w", err)
		}
		cctx, ccancel := context.WithTimeout(ctx, 30*time.Second)
		got, err := rec.waitFor(cctx, func(ev playback.Event) bool {
			return ev.Kind == playback.EventState && ev.Status.State == playback.StatePlaying && ev.Status.UpdatedAt.After(tk)
		})
		ccancel()
		if err != nil {
			return fmt.Errorf("crash recovery: %w", err)
		}
		ni, _ := eng.Host()
		step("crash → resumed", "latency", got.at.Sub(tk).Round(time.Millisecond), "killed_at", pos.Round(100*time.Millisecond),
			"resumed_at", got.ev.Status.Position.Round(100*time.Millisecond), "new_pid", ni.PID)
		info = ni
		time.Sleep(2 * time.Second)
	}

	if !gap {
		return nil
	}
	// 4. Gap: preload the next track, play into the end of this one, start
	// the next on the Ended event, as the coordinator does.
	next, err := resolve(ctx, api, sf.ID, nextID, nextTerm)
	if err != nil {
		return err
	}
	tp := time.Now()
	if err := eng.Preload(ctx, next.ID); err != nil {
		return fmt.Errorf("preload: %w", err)
	}
	step("preload next", "id", next.ID, "name", fmt.Sprintf("%q", next.Attributes.Name), "took", time.Since(tp).Round(time.Millisecond))
	dur := eng.Status().Duration
	if err := eng.Seek(ctx, dur-6*time.Second); err != nil {
		return fmt.Errorf("seek to end: %w", err)
	}
	rec.drain()
	ended, err := rec.waitFor(ctx, func(ev playback.Event) bool {
		return ev.Kind == playback.EventState && ev.Status.State == playback.StateEnded
	})
	if err != nil {
		return fmt.Errorf("waiting for end: %w", err)
	}
	tn := time.Now()
	if err := eng.Play(ctx, next.ID, 0); err != nil {
		return fmt.Errorf("play next: %w", err)
	}
	warm := time.Since(tn)
	st = eng.Status()
	step("track gap", "ended_event→audio", st.UpdatedAt.Sub(ended.at).Round(time.Millisecond), "play_call→audio", warm.Round(time.Millisecond))
	time.Sleep(3 * time.Second)
	s = eng.Status()
	step("next track", "state", s.State, "position", (s.Position + time.Since(s.UpdatedAt)).Round(100*time.Millisecond), "duration", s.Duration.Round(time.Millisecond))
	memory("memory after two tracks", info.PID)
	if err := eng.Stop(ctx); err != nil {
		return err
	}
	return nil
}

// outputDevice is the -device flag: the output every mode plays to
// (deviceHelp).
var outputDevice string

func resolve(ctx context.Context, api *applemusic.Client, sf, id, term string) (applemusic.Resource[applemusic.SongAttributes], error) {
	if id != "" {
		return applemusic.Resource[applemusic.SongAttributes]{ID: id}, nil
	}
	songs, err := api.SearchSongs(ctx, sf, term, 1)
	if err != nil {
		return applemusic.Resource[applemusic.SongAttributes]{}, err
	}
	if len(songs) == 0 {
		return applemusic.Resource[applemusic.SongAttributes]{}, fmt.Errorf("no songs for %q", term)
	}
	return songs[0], nil
}

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

// signIn builds an Apple Music client from MUSIC_USER_TOKEN (env or
// envFile) and validates it, returning the storefront.
func signIn(ctx context.Context, envFile string) (*applemusic.Client, applemusic.Resource[applemusic.StorefrontAttributes], error) {
	var none applemusic.Resource[applemusic.StorefrontAttributes]
	mut := os.Getenv("MUSIC_USER_TOKEN")
	if mut == "" {
		v, err := readEnv(envFile, "MUSIC_USER_TOKEN")
		if err != nil {
			return nil, none, err
		}
		mut = v
	}
	if mut == "" {
		return nil, none, fmt.Errorf("MUSIC_USER_TOKEN not set (env or %s)", envFile)
	}
	api := applemusic.New(&http.Client{Transport: licences}, devtoken.NewWebPlayer(nil), func() string { return mut })
	sf, err := api.Storefront(ctx)
	if err != nil {
		return nil, none, fmt.Errorf("validate MUT: %w", err)
	}
	step("storefront (MUT valid)", "id", sf.ID)
	return api, sf, nil
}
