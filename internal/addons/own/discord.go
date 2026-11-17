package own

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/discord"
)

// The Discord add-on shows the song the user plays as their Discord activity
// (#302). Sona's process used to do it; now this one does, in the OS sandbox,
// with the one Unix socket exception code gives it (docs/security-review.md,
// "Add-ons"): the endpoints of Discord's IPC Sona found (discord.EnvSockets),
// and no network.
//
// It knows of the music only what any add-on is told: the `status` and
// `now-playing` topics, which say nothing of a private listen or a preview
// (docs/addons.md, "Privacy"). Such a listen is an idle status, and so clears
// the activity.

// discordSetup is what the Discord add-on does once hello is answered:
// subscribe, and keep the activity at what the topics say. paths are the
// endpoints of Discord's IPC it may dial; now is the clock.
func discordSetup(paths []string, now func() time.Time) Setup {
	return func(ctx context.Context, c *Client, hello cli.HelloResult) error {
		id := hello.Credentials[cli.CredApplicationID]
		if id == "" {
			// A build without Discord's application ID has nothing to show,
			// and Sona doesn't start the add-on for it; staying up and idle
			// keeps a start by hand from looking like a crash.
			c.log.Warn("this build has no Discord application ID")
			return nil
		}
		rep := newConnectionReporter(c)
		pres := discord.NewPresence(discord.NewAt(id, paths), rep.report, c.log)
		go pres.Run()
		go rep.run()
		c.OnStop(func() {
			pres.Stop()
			rep.stop()
		})

		// Both topics are read on the reading goroutine, one after the other:
		// the plan needs no lock.
		var plan discordPlan
		send := func() {
			if w, ok := plan.want(); ok {
				pres.Set(w)
			}
		}
		c.OnEvent(cli.TopicStatus, func(data json.RawMessage) {
			var s cli.Status
			if json.Unmarshal(data, &s) != nil {
				return
			}
			plan.status(s, now())
			send()
		})
		c.OnEvent(cli.TopicNowPlaying, func(data json.RawMessage) {
			var n cli.NowPlaying
			if json.Unmarshal(data, &n) != nil {
				return
			}
			plan.nowPlaying(n)
			send()
		})
		return c.Subscribe(ctx, cli.TopicStatus, cli.TopicNowPlaying)
	}
}

// discordSockets are the endpoints of Discord's IPC Sona gave this process.
func discordSockets() []string { return discord.SocketsFromEnv(os.Getenv(discord.EnvSockets)) }

// discordPlan is what the topics have said of the listen, and so what the
// activity is. The zero value is nothing playing.
type discordPlan struct {
	state    string // the last status's state
	track    *cli.Track
	position float64   // seconds into track, as of at
	at       time.Time // when the status that said so arrived
	// known: state and position are those of track. A track that now-playing
	// announced before any status has said how far it is shows nothing, rather
	// than an activity that starts at the wrong time.
	known bool
}

// status takes in a status that arrived at now.
func (p *discordPlan) status(s cli.Status, now time.Time) {
	p.state, p.track, p.position, p.at, p.known = s.State, s.Track, s.Position, now, true
}

// nowPlaying takes in a change of the song that plays.
func (p *discordPlan) nowPlaying(n cli.NowPlaying) {
	switch {
	case n.Track == nil:
		// Nothing plays, or the listen is hidden: nothing is shown.
		*p = discordPlan{}
	case p.track == nil || p.track.ID != n.Track.ID:
		p.track, p.known = n.Track, false
	}
}

// want is what Discord should show. ok is false while the next song loads, or
// is known by its name only: the presence stays as it is so that it doesn't
// flicker, as the one in Sona's process did.
func (p *discordPlan) want() (w discord.Want, ok bool) {
	w.Enabled = true
	switch {
	case p.state == "loading" || (p.track != nil && !p.known):
		return w, false
	case p.state != "playing" || p.track == nil:
		return w, true // paused, stopped, failed or hidden: cleared
	}
	t := p.track
	a := &discord.Activity{Title: t.Title, Artist: t.Artist, Album: t.Album, ArtworkURL: discordArt(t.Artwork)}
	a.Start = p.at.Add(-time.Duration(p.position * float64(time.Second)))
	if t.Duration > 0 {
		a.End = a.Start.Add(time.Duration(t.Duration * float64(time.Second)))
	}
	w.Activity = a
	return w, true
}

// discordArt is the artwork URL for Discord, which fetches it itself: only
// https URLs on Apple's public artwork CDN qualify, never anything that could
// carry credentials or point Discord somewhere else.
func discordArt(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".mzstatic.com") || host == ".mzstatic.com" || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	return raw
}

// connectionReporter tells Sona, over the channel, whether Discord is
// connected after each attempt to reach it (Settings → Connections says so),
// and Sona restarts the add-on when Discord has come up in a socket the
// sandbox did not show it. The latest report wins.
type connectionReporter struct {
	c    *Client
	ch   chan bool
	quit chan struct{}
	done chan struct{}
}

func newConnectionReporter(c *Client) *connectionReporter {
	return &connectionReporter{c: c, ch: make(chan bool, 1), quit: make(chan struct{}), done: make(chan struct{})}
}

// report queues connected without blocking the presence's worker.
func (r *connectionReporter) report(connected bool) {
	for {
		select {
		case r.ch <- connected:
			return
		default:
			select {
			case <-r.ch:
			default:
			}
		}
	}
}

func (r *connectionReporter) run() {
	defer close(r.done)
	for {
		select {
		case <-r.quit:
			return
		case v := <-r.ch:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := r.c.Call(ctx, cli.AddonMethodConnection, cli.ConnectionParams{Connected: v}, nil); err != nil {
				r.c.log.Debug("connection report", "err", err)
			}
			cancel()
		}
	}
}

func (r *connectionReporter) stop() {
	close(r.quit)
	<-r.done
}
