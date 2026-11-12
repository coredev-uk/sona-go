// Package video is the music video feature: a catalogue music video's page
// (/music-video/{id}) and the video view, the playing music video's picture
// over the whole window with its own transport.
//
// The pictures come from the CDM host child by way of the engine's
// video.Surface (memory only: the latest picture, never written to disk) and
// are drawn by kit.VideoSurface; this package decides when the view shows,
// what surrounds the picture and what its controls do. Every command goes
// through services.PlaybackService off the main thread, and its failure comes
// back as a toast titled by the command and worded by i18n.ErrorText.
package video

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// videoPath is the page's route.
const videoPath = "/music-video/{id}"

// catalogAPI is what the page and the view's problem box ask of
// CatalogService.
type catalogAPI interface {
	MusicVideo(ctx context.Context, id string) (services.CatalogVideo, error)
	OpenInAppleMusic(url string) error
}

// videoQueue is how the page plays: kit.QueueCommands' PlayVideos and
// PlaySongs.
type videoQueue interface {
	PlayVideos(videos []services.MusicVideo, start int, opened func())
	PlaySongs(songs []services.Song, start int, from services.PlayFrom)
}

// video is the feature's state.
type video struct {
	env *module.Env
	cat catalogAPI
	q   videoQueue
	// caps says what this platform plays: music videos (Video) and whether
	// Sona sets the volume (Volume); known is false until it has answered
	// (store.UseCapabilities): nothing that depends on it shows before.
	caps func() (playback services.PlaybackCapabilities, known bool)
	feed *kit.VideoFeed
	// playBtn is the page's Play button, which the video view returns the
	// focus to.
	playBtn ui.Handle

	// The page: the header's action menu and the two shelves' state.
	menus  *kit.ActionMenus
	byline kit.CoverShelfState
	more   kit.CoverShelfState
	scroll ui.ScrollState
	pageW  float32 // the page's width as laid out last frame
	shown  string  // the video the page last built, whose scroll place it keeps

	view viewState
	// tr is the playback commands; a failure is a toast.
	tr kit.Transport
}

// New is the video feature's module.
func New(env *module.Env) module.Module {
	cat := services.Service[services.CatalogService](env.Core)
	pb := services.Service[services.PlaybackService](env.Core)
	v := newVideo(env, cat, pb, func() (services.PlaybackCapabilities, bool) {
		r := store.UseCapabilities(env.Queries, env.Core)
		return r.Data.Playback, r.HasData
	})
	if env.Queue != nil {
		v.q = env.Queue
	}
	return v.module()
}

func newVideo(env *module.Env, cat catalogAPI, svc kit.PlaybackService, caps func() (services.PlaybackCapabilities, bool)) *video {
	v := &video{env: env, cat: cat, caps: caps}
	v.tr = kit.Transport{Svc: svc, Play: env.Play}
	if env.Video != nil && env.Video.Frames() != nil {
		v.feed = kit.NewVideoFeed(env.Video.Frames(), env.Post)
	}
	if env.Actions != nil {
		v.menus = kit.NewActionMenus(env.Actions, nil)
	}
	return v
}

func (v *video) module() module.Module {
	return module.Module{
		ID: "video",
		Routes: []module.Route{{
			Pattern: videoPath,
			Title:   func(*ui.Route) string { return i18n.T("Music video") },
			Build:   v.page,
		}},
		Slots: module.Slots{Layer: v.layer, LayerOpen: v.layerOpen},
	}
}

// keyVideo is v0.2.0's keys.catalogVideo: ["catalog", "music-video", id].
func keyVideo(id string) store.Key { return store.Key{"catalog", "music-video", id} }

// videoQuery is the page's data, shared with the view's "Watch in Apple
// Music" (the same key, so one fetch serves both).
func (v *video) videoQuery(id string) store.Result[services.CatalogVideo] {
	return store.Use(v.env.Queries, keyVideo(id), func(ctx context.Context) (services.CatalogVideo, error) {
		return v.cat.MusicVideo(ctx, id)
	})
}

// openInAppleMusic opens a music video's page on music.apple.com in the
// browser; its failure is a toast.
func (v *video) openInAppleMusic(url string) {
	title := i18n.T("Couldn't open Apple Music")
	go func() {
		if err := v.cat.OpenInAppleMusic(url); err != nil {
			kit.ToastFailure(title, err)
		}
	}()
}

// seekBy seeks by delta from the live position, within the video, unless it
// cannot seek.
func (v *video) seekBy(delta time.Duration, now time.Time) {
	v.tr.SeekBy(delta, now, func() bool { return !v.env.Play.State().Controls.Seek })
}

// playVideos plays music videos from start as the queue and opens the video
// view once playback started, from the control bound to opener (nil: none),
// which gets the focus back when the view closes.
func (v *video) playVideos(videos []services.MusicVideo, start int, opener *ui.Handle) {
	v.q.PlayVideos(videos, start, func() { v.env.Video.OpenFrom(opener) })
}
