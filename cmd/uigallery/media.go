package main

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/playback/video"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// galleryFile is the motion policy of the sections that play. Motion needs a
// decoder and a file: SONA_GALLERY_MOTION names an MP4 (a generated clip:
// gst-launch-1.0 videotestsrc num-buffers=90 pattern=smpte ! x264enc ! mp4mux
// ! filesink location=clip.mp4); without it covers show their stills, as where
// motion is off.
type galleryFile string

func (f galleryFile) Local(context.Context, string, int, string) (string, error) {
	return string(f), nil
}

var (
	galleryOnce   sync.Once
	galleryPolicy *kit.MotionPolicy
	galleryFeed   *kit.VideoFeed
)

// galleryPost runs a func on the window's main thread and redraws it. The
// gallery's test has no window and nothing to run it: it drops the func.
func galleryPost(g *gallery) store.Poster {
	return func(fn func()) {
		if g.win != nil {
			g.win.Update(fn)
		}
	}
}

// galleryMedia is the motion policy and the music video feed (one grey-ramp
// picture) of the sections that play.
func galleryMedia(g *gallery) (*kit.MotionPolicy, *kit.VideoFeed) {
	galleryOnce.Do(func() {
		var files media.MotionFiles
		if f := os.Getenv("SONA_GALLERY_MOTION"); f != "" {
			files = galleryFile(f)
		}
		galleryPolicy = kit.NewMotionPolicy(nil, galleryPost(g), kit.MotionOptions{Files: files, Art: g.art})
		surface := video.NewSurface()
		w, h := 160, 90
		pic := make([]byte, media.I420Len(w, h))
		for y := range h {
			for x := range w {
				pic[y*w+x] = byte(40 + 180*x/w)
			}
		}
		for i := w * h; i < len(pic); i++ {
			pic[i] = 128
		}
		surface.Show(w, h, 0, pic)
		galleryFeed = kit.NewVideoFeed(surface, galleryPost(g))
		galleryFeed.Start()
	})
	return galleryPolicy, galleryFeed
}

func init() {
	register(section{name: "Media / MotionCover, TileMotion", build: func(c *ui.Context, g *gallery) {
		p, _ := galleryMedia(g)
		ui.Row(c).Gap(16).Children(func() {
			kit.MotionCover(c.Key("square"), p, g.art, "/art/gallery-1/300x300.jpg", "/motion/gallery-1/720.mp4", 120, kit.MotionOpts{})
			kit.MotionCover(c.Key("round"), p, g.art, "/art/gallery-1/300x300.jpg", "/motion/gallery-1/720.mp4", 120, kit.MotionOpts{Round: true})
			// A tile whose motion plays while it is hovered or focused, after 400 ms.
			tile := ui.Box(c.Key("tile")).Size(120, 120)
			hot := tile.Hovered() || tile.FocusWithin()
			tile.Children(func() {
				kit.Artwork(c.Key("art"), g.art, "/art/gallery-1/300x300.jpg", 120, kit.ArtOpts{Edge: true})
				kit.TileMotion(c.Key("motion"), p, "/motion/gallery-1/720.mp4", 120, kit.TileMotionOpts{Hot: hot})
			})
		})
	}})

	register(section{name: "Media / VideoSurface, FluidArtwork", build: func(c *ui.Context, g *gallery) {
		_, feed := galleryMedia(g)
		ui.Row(c).Gap(16).Children(func() {
			ui.Box(c.Key("surface")).Size(240, 135).Background(theme.C("surface")).Children(func() {
				kit.VideoSurface(c.Key("v"), feed, kit.VideoOpts{A11yLabel: "Music video"})
			})
			ui.Box(c.Key("fluid")).Width(160).Children(func() {
				kit.FluidArtwork(c.Key("f"), g.art, "/art/gallery-1/300x300.jpg", 160, kit.ArtOpts{Aspect: 16.0 / 9, Edge: true})
			})
			ui.Box(c.Key("round")).Width(100).Children(func() {
				kit.FluidArtwork(c.Key("r"), g.art, "/art/gallery-1/300x300.jpg", 100, kit.ArtOpts{Round: true})
			})
		})
	}})

	register(section{name: "Media / PersonAvatar and Avatar", build: func(c *ui.Context, g *gallery) {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			for i, n := range []string{"Thom Yorke", "Nigel Godrich", "The Bad Plus", "米津玄師", "Madonna", "50 Cent", "$$$", ""} {
				kit.PersonAvatar(c.Key(i), g.art, n, "", 40)
			}
			kit.PersonAvatar(c.Key("pic"), g.art, "Thom Yorke", "/art/gallery-1/300x300.jpg", 40)
		})
		ui.Row(c.Key("avatars")).Gap(16).AlignItems(ui.Center).Margin(16, 0, 0, 0).Children(func() {
			kit.Avatar(c.Key("sm"), g.art, kit.AvatarOpts{Fallback: "AB", Size: kit.AvatarSmall})
			kit.Avatar(c.Key("md"), g.art, kit.AvatarOpts{Fallback: "AB", Badge: true})
			kit.Avatar(c.Key("lg"), g.art, kit.AvatarOpts{Fallback: "AB", Badge: true, Size: kit.AvatarLarge})
			kit.AvatarGroup(c.Key("group"), g.art, []kit.AvatarOpts{{Fallback: "A"}, {Fallback: "B"}, {Fallback: "C"}}, 4)
			kit.SonaMark(c.Key("mark"), 0)
		})
	}})

	register(section{name: "Media / Backdrop", build: func(c *ui.Context, g *gallery) {
		ui.Row(c).Gap(16).Children(func() {
			for i, tone := range []kit.BackdropTone{kit.ToneSurface, kit.ToneGround} {
				kit.Backdrop(c.Key(i), g.art, kit.BackdropOpts{Src: "/art/gallery-1/backdrop.jpg", Color: "#c2553a", Tone: tone}, func() {
					ui.Text(c, "Title over a backdrop").FontSize(16).FontWeight(600).TextColor(theme.C("ink"))
					ui.Text(c, "Text keeps 4.5:1 over any cover").TextColor(theme.C("ink-muted"))
				}).Size(260, 96).Padding(12)
			}
		})
	}})

	register(section{name: "Media / Equaliser, SonaMark, GradientText", build: func(c *ui.Context, g *gallery) {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			kit.EqualiserBars(c.Key("playing"), true, nil)
			kit.EqualiserBars(c.Key("paused"), false, nil)
		})
		for i, p := range []float32{0, 0.3, 0.6, 1} {
			kit.GradientText(c.Key(i), "A word being sung", kit.GradientOpts{
				Size: 28, Weight: 700, Base: theme.C("ink-muted"), Fill: theme.C("amber-ink"), Progress: p,
			})
		}
		words := []lyrics.Word{{Begin: 0, End: time.Second, Text: "Held "}, {Begin: time.Second, End: 3 * time.Second, Text: "note"}}
		s := kit.NewSweepText(words, 28, 700, 36, 300)
		ui.Box(c.Key("sweep")).Size(s.Width(), s.Height()).Draw(func(p *ui.Painter, r ui.Rect) {
			t := time.Duration(p.Now().UnixNano()) % (4 * time.Second)
			if s.Paint(p, r.X, r.Y, theme.C("ink-muted"), theme.C("amber-ink"), 1, t, false) {
				p.AnimationFrame()
			}
		})
	}})
}
