package library

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// videosPageSize is how many music videos a request asks for (v0.2.0 read
// the whole list, which rarely holds more than a page, 100 at a time).
const videosPageSize = 100

// videosSkeleton is how many loading tiles the page shows while it reads.
const videosSkeleton = 6

// keyMusicVideos is the query of the library's music videos.
var keyMusicVideos = store.Key{"library", "music-videos"}

// videosPage is the Music videos page's view state.
type videosPage struct {
	pageView
	cover kit.CoverState
	menus *kit.ActionMenus
}

// videosPage is MusicVideosPage: Apple's music videos of the library (they
// need the network), each opening its page and, where videos play, playing.
func (l *library) videosPage(c *ui.Context, r *ui.Route) {
	pg := &l.videos
	q := store.Use(l.env.Queries, keyMusicVideos, l.musicVideos, store.Options{Stale: 10 * time.Minute})
	auth := l.authStatus()
	out := auth.HasData && account.IsSignedOut(auth.Data.State)

	subline := ""
	if q.HasData && len(q.Data) > 0 {
		// translators: {0} is the number of music videos.
		subline = i18n.T("{0, plural, one {# music video} other {# music videos}}", "0", len(q.Data))
	}
	l.scrollPage(c, &pg.pageView, "videos", func(pane *kit.Pane) {
		kit.PageHeader(c, i18n.T("Music videos"), subline, nil)
		switch {
		case out:
			kit.EmptyState(c, i18n.T("Sign in to see your music videos"),
				i18n.T("Music videos you add in Apple Music are listed here after you sign in."),
				kit.EmptyOpts{Frame: kit.FrameBox, Action: l.signIn()})
			return
		case q.Err != nil:
			kit.ErrorBlock(c, i18n.T("Couldn't load your music videos from Apple Music"), q.Err, kit.ErrorBlockOpts{
				OnRetry:  func() { l.env.Queries.Invalidate(keyMusicVideos) },
				Retrying: q.Fetching,
			})
			return
		case q.HasData && len(q.Data) == 0:
			kit.EmptyState(c, i18n.T("No music videos in your library"),
				i18n.T("Music videos you add in Apple Music on any device are listed here."),
				kit.EmptyOpts{Frame: kit.FrameBox})
			return
		}
		count, at := videosSkeleton, func(int) *kit.CoverItem { return nil }
		if q.HasData {
			videos := q.Data
			canPlay := l.canPlayVideo()
			count = len(videos)
			at = func(i int) *kit.CoverItem {
				t := l.videoTile(videos, i, canPlay)
				return &t
			}
		}
		kit.CoverGrid(c, &pg.cover, count, at, kit.CoverGridOpts{
			CoverOpts: kit.CoverOpts{
				Label: i18n.T("Music videos in your library"), Min: 240, Video: true,
				Art: l.env.Art, Motion: l.env.Motion, Restricted: l.restricted(),
				Menus: l.itemMenus(&pg.menus, &pg.cover.Sel),
			},
			Pane: pane,
		})
	})
}

// musicVideos reads the whole list, a page at a time; off the main thread.
func (l *library) musicVideos(ctx context.Context) ([]services.MusicVideo, error) {
	first, err := l.lib.MusicVideos(ctx, 0, videosPageSize)
	if err != nil {
		return nil, err
	}
	items := first.Items
	for len(items) < first.Total {
		next, err := l.lib.MusicVideos(ctx, len(items), videosPageSize)
		if err != nil {
			return nil, err
		}
		if len(next.Items) == 0 {
			break
		}
		items = append(items, next.Items...)
	}
	return items, nil
}

// canPlayVideo: this platform and build play music videos
// (Env.Players.CanPlayVideo).
func (l *library) canPlayVideo() bool { return l.env.Players.CanPlayVideo() }

// videoTile is the tile of videos[i] (v0.2.0's videoTile): its page when one
// exists, the artist and length under the title and, where videos play, a
// play control and the video as the tile's menu target. Playing starts this
// video alone as the queue (shared/items/play.ts playVideo).
func (l *library) videoTile(videos []services.MusicVideo, i int, canPlay bool) kit.CoverItem {
	v := videos[i]
	sub := []string{}
	if v.Artist != "" {
		sub = append(sub, v.Artist)
	}
	if v.DurationMs > 0 {
		sub = append(sub, i18n.FormatDuration(time.Duration(v.DurationMs)*time.Millisecond))
	}
	tile := kit.CoverItem{
		Key:      v.ID,
		Title:    v.Name,
		Subtitle: strings.Join(sub, " · "),
		Art:      v.Art.Medium,
		Video:    true,
		Explicit: v.Explicit,
	}
	id := v.CatalogID
	if id == "" {
		id = v.ID
	}
	l.openTo(&tile, "/music-video/"+id)
	if canPlay {
		item := kit.MusicVideoItem(v)
		tile.Item = &item
		tile.Play = &kit.CoverPlay{Label: v.Name, RunFrom: func(opener *ui.Handle) {
			l.env.Queue.PlayVideos([]services.MusicVideo{v}, 0, func() { l.env.Video.OpenFrom(opener) })
		}}
	}
	return tile
}
