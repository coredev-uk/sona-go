package services

import (
	"cmp"
	"context"
	"log/slog"
	"time"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/mpris"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// mprisArtSize is the artwork edge handed to desktop shells.
const mprisArtSize = 512

// MPRISPlayer adapts the coordinator to mpris.Player.
type MPRISPlayer struct {
	log   *slog.Logger
	c     *coordinator.Coordinator
	raise func()
	quit  func()
}

// NewMPRISPlayer returns the adapter; raise brings the main window to the
// front and quit exits the app.
func NewMPRISPlayer(c *coordinator.Coordinator, raise, quit func(), log *slog.Logger) *MPRISPlayer {
	return &MPRISPlayer{c: c, raise: raise, quit: quit, log: log}
}

func (p *MPRISPlayer) PlayPause(ctx context.Context) error {
	p.log.Info("playback: command", "cmd", "toggle", "from", "mpris")
	return p.c.Toggle(ctx)
}

func (p *MPRISPlayer) Play(ctx context.Context) error {
	p.log.Info("playback: command", "cmd", "resume", "from", "mpris")
	return p.c.Resume(ctx)
}

func (p *MPRISPlayer) Pause(ctx context.Context) error    { return p.c.Pause(ctx) }
func (p *MPRISPlayer) Next(ctx context.Context) error     { return p.c.Next(ctx) }
func (p *MPRISPlayer) Previous(ctx context.Context) error { return p.c.Previous(ctx) }

// Stop pauses and rewinds; it keeps the queue (the app's Stop empties it,
// which a media key should not do). A live station has nothing to rewind.
func (p *MPRISPlayer) Stop(ctx context.Context) error {
	if err := p.c.Pause(ctx); err != nil {
		return err
	}
	if !p.c.Snapshot().Controls.Seek {
		return nil
	}
	return p.c.Seek(ctx, 0)
}

func (p *MPRISPlayer) Seek(ctx context.Context, offset time.Duration) error {
	snap := p.c.Snapshot()
	pos := position(snap, time.Now()) + offset
	if snap.Duration > 0 && pos >= snap.Duration {
		return p.c.Next(ctx)
	}
	return p.c.Seek(ctx, max(pos, 0))
}

func (p *MPRISPlayer) SetPosition(ctx context.Context, trackID string, pos time.Duration) error {
	if cur := p.c.Snapshot().Current; cur == nil || cur.Item.ID != trackID {
		return nil
	}
	return p.c.Seek(ctx, pos)
}

func (p *MPRISPlayer) SetVolume(ctx context.Context, v float64) error { return p.c.SetVolume(ctx, v) }

// SetLoopStatus and SetShuffle refuse while a station plays (the snapshot
// reports them off, and a client writing them would otherwise change what
// plays after it).
func (p *MPRISPlayer) SetLoopStatus(_ context.Context, l mpris.LoopStatus) error {
	if err := p.c.Snapshot().Refusal(func(c coordinator.Controls) bool { return c.Repeat }); err != nil {
		return err
	}
	r := coordinator.RepeatOff
	switch l {
	case mpris.LoopTrack:
		r = coordinator.RepeatOne
	case mpris.LoopPlaylist:
		r = coordinator.RepeatAll
	}
	return p.c.SetRepeat(r)
}

func (p *MPRISPlayer) SetShuffle(_ context.Context, on bool) error {
	if err := p.c.Snapshot().Refusal(func(c coordinator.Controls) bool { return c.Shuffle }); err != nil {
		return err
	}
	p.c.SetShuffle(on)
	return nil
}

func (p *MPRISPlayer) Raise(context.Context) error { p.raise(); return nil }
func (p *MPRISPlayer) Quit(context.Context) error  { p.quit(); return nil }

func (p *MPRISPlayer) Snapshot() mpris.Snapshot { return mprisSnapshot(p.c.Snapshot(), "") }

// position extrapolates snap's position to now.
func position(snap coordinator.Snapshot, now time.Time) time.Duration {
	pos := snap.Position
	if snap.State == playback.StatePlaying && !snap.At.IsZero() {
		pos += now.Sub(snap.At)
	}
	if snap.Duration > 0 {
		pos = min(pos, snap.Duration)
	}
	return pos
}

// mprisSnapshot maps the coordinator state to MPRIS. artURL is the
// current item's cached artwork (file://), if known.
func mprisSnapshot(snap coordinator.Snapshot, artURL string) mpris.Snapshot {
	out := mpris.Snapshot{
		Volume:     snap.Volume,
		Position:   snap.Position,
		UpdatedAt:  snap.At,
		CanControl: true,
		CanPlay:    len(snap.Queue) > 0,
	}
	// Shuffle and repeat don't apply to a station: it reports them off
	// (the settings come back with the next song).
	out.Shuffle = snap.Shuffle && snap.Controls.Shuffle
	switch {
	case !snap.Controls.Repeat:
		out.Loop = mpris.LoopNone
	case snap.Repeat == coordinator.RepeatOne:
		out.Loop = mpris.LoopTrack
	case snap.Repeat == coordinator.RepeatAll:
		out.Loop = mpris.LoopPlaylist
	default:
		out.Loop = mpris.LoopNone
	}
	switch {
	case snap.Current == nil || snap.State == playback.StateError:
		out.Status = mpris.StatusStopped
	case snap.State == playback.StatePlaying || snap.State == playback.StateLoading:
		out.Status = mpris.StatusPlaying
	default:
		out.Status = mpris.StatusPaused
	}
	if cur := snap.Current; cur != nil {
		dur := snap.Duration
		if dur == 0 {
			dur = cur.Item.Duration
		}
		out.Track = &mpris.Track{
			ID: cur.Item.ID, Title: cur.Item.Title, Artist: cur.Item.Artist,
			Album: cur.Item.Album, Length: dur, ArtURL: artURL,
		}
		if on := snap.OnAir; on.Title != "" {
			// What a broadcaster has on air, from the station (its name
			// as the album, and as the artist when the stream names none).
			out.Track.Title, out.Track.Artist, out.Track.Album = on.Title, cmp.Or(on.Artist, cur.Item.Title), cur.Item.Title
		}
		out.CanPause = true
		out.CanSeek = snap.Controls.Seek && dur > 0
		out.CanGoPrevious = snap.Controls.Previous
		out.CanGoNext = snap.Controls.Next
	}
	return out
}

// mprisArtTemplate is the artwork template MPRIS shows for snap: the cover
// of the song a local broadcaster has on air when the catalog has it, else
// the current item's.
func mprisArtTemplate(snap coordinator.Snapshot) string {
	if snap.Current == nil {
		return ""
	}
	if snap.OnAir.Title != "" && snap.OnAir.Art != "" {
		return snap.OnAir.Art
	}
	return snap.Current.Item.Artwork
}

// runMediaControls pushes coordinator snapshots to srv until quit closes. Artwork
// is resolved to a local file off the update path; the track is
// republished with it once it is on disk.
func runMediaControls(srv MediaControls, snaps <-chan coordinator.Snapshot, quit <-chan struct{}, art *artwork.Cache, log *slog.Logger) {
	type artDone struct {
		tmpl, url string
	}
	var (
		tmpl, artURL string
		last         coordinator.Snapshot
		arts         = make(chan artDone, 8)
	)
	for {
		select {
		case <-quit:
			return
		case snap, ok := <-snaps:
			if !ok {
				return
			}
			last = snap
			cur := mprisArtTemplate(snap)
			if cur != tmpl {
				tmpl, artURL = cur, ""
				if cur != "" && art != nil {
					t := cur
					diag.Go(log, "mpris artwork", func() {
						ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()
						p, err := art.LocalPath(ctx, t, mprisArtSize, mprisArtSize)
						if err != nil {
							log.Debug("mpris: artwork", "err", err)
							return
						}
						select {
						case arts <- artDone{t, "file://" + p}:
						default:
						}
					})
				}
			}
			srv.Update(mprisSnapshot(snap, artURL))
		case a := <-arts:
			if a.tmpl == tmpl {
				artURL = a.url
				srv.Update(mprisSnapshot(last, artURL))
			}
		}
	}
}
