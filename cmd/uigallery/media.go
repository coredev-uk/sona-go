//go:build gallery_pending

package main

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Media components: persons, avatars, backdrops, covers that move, the
// equaliser and gradient text. This file waits for PR 0.3's cmd/uigallery
// (register, section, gallery): the lead removes the build tag at rebase.
func init() {
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
