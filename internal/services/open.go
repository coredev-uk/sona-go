package services

import (
	"log/slog"
	"sync"

	"github.com/coredev-uk/sona-go/internal/app"
)

// OpenTarget is the "app:open" payload: an Apple Music link Sona was
// launched with (argv or a forwarded second instance). Kind is song,
// album, artist, playlist (catalog "pl.…"), library-playlist ("p.…"),
// library-album ("l.…"), music-video, station ("ra.…") or curator;
// AlbumID is set for a song shared within its album.
type OpenTarget struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	AlbumID    string `json:"albumId"`
	Storefront string `json:"storefront"`
}

// opener turns activations into "app:open" events. Targets that arrive
// before the UI is listening (first paint) are held and delivered
// then.
type opener struct {
	emit  Emitter
	focus func()
	log   *slog.Logger

	mu      sync.Mutex
	ready   bool
	pending []OpenTarget
}

// targets extracts the Apple Music links among args.
func targets(args []string, log *slog.Logger) []OpenTarget {
	var out []OpenTarget
	for _, a := range args {
		if !app.IsMusicURL(a) {
			continue
		}
		t, err := app.ParseURL(a)
		if err != nil {
			log.Info("ignoring link", "err", err)
			continue
		}
		out = append(out, OpenTarget{Kind: string(t.Kind), ID: t.ID, AlbumID: t.AlbumID, Storefront: t.Storefront})
	}
	return out
}

// activate handles a launch: it raises the window (forwarded launches
// only, unless they ask to stay hidden, like a login autostart while Sona
// already runs; the first launch shows it anyway) and emits its links.
func (o *opener) activate(act app.Activation, forwarded bool) {
	if forwarded && !act.Hidden() && o.focus != nil {
		o.focus()
	}
	ts := targets(act.Args, o.log)
	o.mu.Lock()
	if !o.ready {
		o.pending = append(o.pending, ts...)
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	for _, t := range ts {
		o.emit.Emit(EventAppOpen, t)
	}
}

// markReady delivers held targets; later ones are emitted at once.
func (o *opener) markReady() {
	o.mu.Lock()
	o.ready = true
	ts := o.pending
	o.pending = nil
	o.mu.Unlock()
	for _, t := range ts {
		o.emit.Emit(EventAppOpen, t)
	}
}
