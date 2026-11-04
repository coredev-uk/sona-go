package player

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// nowPlayingLayoutFor is how the view is laid out in a box of w × h DIPs
// (NowPlayingView.tsx's useNowPlayingLayout): the cover beside the pane, or,
// in a column under 880 wide, the song as a header over the pane. The cover
// comes from a fixed set, so Go serves the art at its exact size.
func nowPlayingLayoutFor(w, h float32) nowPlayingLayout {
	stacked := w < 880
	byWidth := float32(320)
	if w >= 1000 {
		byWidth = 400
	}
	byHeight := float32(240)
	switch {
	case h >= 720:
		byHeight = 400
	case h >= 640:
		byHeight = 320
	}
	cover := min(byWidth, byHeight)
	if stacked {
		cover = 168
	}
	return nowPlayingLayout{stacked: stacked, cover: cover}
}

// measureLayout is the layout from the view's own box as last drawn (not the
// window: the sidebar takes 232 of it or none); before the first frame, from
// the window less the sidebar, top bar and bar.
func (p *player) measureLayout(c *ui.Context) nowPlayingLayout {
	b := p.np.root.Bounds(c)
	w, h := b.W, b.H
	if w <= 0 || h <= 0 {
		ww, wh := c.Size()
		tk := theme.Current().Metric
		w, h = ww-tk["sidebar"]-1, wh-tk["top-bar"]-tk["player-bar"]-2
	}
	return nowPlayingLayoutFor(w, h)
}

// panes are the panes the view offers: a station, a music video or a song
// outside the catalogue has only Up next.
func panesFor(cur *services.QueueItem) []paneKind {
	if lyricsync.SongID(cur) != "" && !cur.Live {
		return []paneKind{paneLyrics, paneQueue}
	}
	return []paneKind{paneQueue}
}

func paneName(k paneKind) string {
	if k == paneLyrics {
		return i18n.T("Lyrics")
	}
	return i18n.T("Up next")
}

// nowPlaying is NowPlayingView: the record and its insert. The cover on a
// wash of its colour, the song's label copy under it, and beside it one pane
// at a time, switched by tabs: the lyric sheet, or Up next (the queue). Each
// pane's own controls sit in a strip at its foot; Close ends the tab row.
// Under 880 of column the song becomes a header over the pane. Escape or Close
// returns to the page.
func (p *player) nowPlaying(c *ui.Context) {
	cur := p.env.Play.State().Current
	if cur == nil {
		// Nothing to show once the queue is cleared.
		p.ui.closeOverlay()
		c.Invalidate()
		return
	}
	first := !p.np.shown
	p.np.shown = true
	if first {
		// Mounted again: scroll and focus start from the sung line.
		p.np.sheet = nil
	}
	lay := p.measureLayout(c)
	panes := panesFor(cur)
	shown := shownPane(p.ui.pane, cur)
	songID := lyricsync.SongID(cur)

	root := ui.Column(c.Key("now-playing")).FillWidth().FillHeight().Grow(1).Basis(0).MinHeight(0)
	root.Children(func() {
		kit.Backdrop(c.Key("wash"), p.env.Art, kit.BackdropOpts{Src: cur.Backdrop, Tone: kit.ToneGround}, nil).
			Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough()
		ui.Row(c.Key("center")).FillWidth().FillHeight().Grow(1).Basis(0).MinHeight(0).Justify(ui.Center).AlignItems(ui.Stretch).Children(func() {
			var body ui.Element
			if lay.stacked {
				body = ui.Column(c.Key("body")).Padding(20, 24, 16)
			} else {
				body = ui.Row(c.Key("body")).Gap(56).Padding(24, 40, 32)
			}
			body.MaxWidth(1280).FillWidth().FillHeight().MinHeight(0).AlignItems(ui.Stretch)
			body.Children(func() {
				if lay.stacked {
					ui.Row(c.Key("header")).FillWidth().Shrink(0).Gap(20).Padding(0, 0, 20).AlignItems(ui.Start).Children(func() {
						p.nowPlayingArt(c, cur, lay.cover)
						ui.Column(c.Key("copy")).Grow(1).MinWidth(0).Padding(4, 0, 0).Children(func() { p.songCopy(c, cur, false) })
						p.closeButton(c)
					})
				} else {
					ui.Column(c.Key("left")).Width(lay.cover).Shrink(0).Gap(24).Clip().Children(func() {
						p.nowPlayingArt(c, cur, lay.cover)
						p.songCopy(c, cur, lay.cover >= 320)
					})
				}
				ui.Column(c.Key("pane")).FillHeight().Grow(1).Basis(0).MinWidth(0).MinHeight(0).Children(func() {
					ui.Row(c.Key("tabs-row")).FillWidth().Shrink(0).Gap(24).PaddingStart(12).AlignItems(ui.Center).Children(func() {
						p.paneTabs(c, panes, shown)
						if !lay.stacked {
							ui.Box(c.Key("grow")).Grow(1)
							p.closeButton(c)
						}
					})
					panel := ui.Column(c.Key("panel")).FillHeight().Grow(1).Basis(0).MinHeight(0).Margin(12, 0, 0, 0)
					if len(panes) > 1 {
						panel.Role(ui.RoleGroup).Label(paneName(shown))
					}
					panel.Children(func() {
						if shown == paneLyrics {
							p.lyricsPane(c, songID)
						} else {
							p.queuePane(c)
						}
					})
				})
			})
		})
	})
	root.Role(ui.RoleGroup).Label(i18n.T("Now playing")).Bind(&p.np.root)
	if !theme.ReducedMotion() {
		root.Transition(ui.ElementTransition{Enter: &ui.Motion{}, Duration: 150 * time.Millisecond})
	}

	if first {
		// Focus starts on the shown pane's tab (Close when there is one pane),
		// or on Full screen when coming back from it.
		switch {
		case p.ui.fromRoom && songID != "":
			p.np.roomToggle.Focus(c)
		case len(panes) > 1 && shown == paneLyrics:
			p.np.tabs[0].Focus(c)
		case len(panes) > 1:
			p.np.tabs[1].Focus(c)
		default:
			p.np.closeBtn.Focus(c)
		}
	}
}

// closeButton is the view's Close.
func (p *player) closeButton(c *ui.Context) {
	if kit.Button(c.Key("close"), "", kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Icon: "x", A11yLabel: i18n.T("Close now playing (Escape)"),
	}).Bind(&p.np.closeBtn).Shrink(0).Clicked() {
		p.ui.closeOverlay()
	}
}

// paneTabs is the switch between the panes: tabs named in words, the shown
// one in ink over a 2 px ink rule. One Tab stop; the arrows move and show,
// Home and End go to the ends. With one pane it is only that pane's heading.
func (p *player) paneTabs(c *ui.Context, panes []paneKind, shown paneKind) {
	if len(panes) < 2 {
		ui.Text(c.Key("heading"), paneName(shown)).FontSize(16).FixedLineHeight(40).FontWeight(600).
			TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3)
		return
	}
	sel := 0
	for i, k := range panes {
		if k == shown {
			sel = i
		}
	}
	tabs := ui.TabsBase(c.Key("pane-tabs"), &sel, len(panes))
	tabs.List.Gap(24).AlignItems(ui.Center)
	tabs.List.Children(func() {
		for i, k := range panes {
			tab := tabs.Tab(i).Height(40).PaddingX(6).Margin(0, -6).Radius(theme.Current().Radius["sm"]).Label(paneName(k)).Bind(&p.np.tabs[i])
			col := theme.C("ink-muted")
			if i == sel || tab.Hovered() {
				col = theme.C("ink")
			}
			tab.Children(func() {
				ui.Text(c.Key("t"+itoa(i)), paneName(k)).FontSize(16).FixedLineHeight(22).FontWeight(600).TextColor(col)
				if i == sel {
					ui.Box(c.Key("rule")).Absolute().Left(6).Right(6).Bottom(2).Height(2).Radius(1).Background(theme.C("ink"))
				}
			})
		}
	})
	if sel != indexOfPane(panes, shown) {
		p.ui.setPane(panes[sel])
	}
}

func indexOfPane(panes []paneKind, k paneKind) int {
	for i, v := range panes {
		if v == k {
			return i
		}
	}
	return 0
}

// nowPlayingArt is the playing song's cover, moving when its album has motion
// artwork; a music video's cover is a 16:9 frame of its art; a local
// broadcaster's the art of the song its stream names on air.
func (p *player) nowPlayingArt(c *ui.Context, cur *services.QueueItem, size float32) {
	s := p.env.Play.State()
	if cur.Video {
		ui.Box(c.Key("art")).Width(size).Children(func() {
			kit.FluidArtwork(c, p.env.Art, cur.Art.Large, size, kit.ArtOpts{Aspect: 16.0 / 9})
		})
		return
	}
	art := cur.Art.Large
	if cur.Live && cur.Broadcast && s.OnAir != nil && s.OnAir.Art.Large != "" {
		art = s.OnAir.Art.Large
	}
	label := ""
	if cur.Album != "" {
		label = i18n.T("{albumName} cover", "albumName", cur.Album)
	}
	// One rendition for every size, so a resize never reloads the video.
	kit.MotionCover(c.Key("art"), p.env.Motion, p.env.Art, art, p.albumMotion(cur.AlbumID), size, kit.MotionOpts{
		VideoSize: 420, Overlay: true, A11yLabel: label,
	})
}

// idPath is the page of an item: "" when it has no ID.
func idPath(prefix, id string) string {
	if id == "" {
		return ""
	}
	return prefix + id
}

// outLink is a line of muted text that links out of the view when the page it
// names is ported (path "": it is plain): the view closes as it goes.
func (p *player) outLink(c *ui.Context, key, text, path string, size, lineH float32, weight int, top float32) {
	t := lineText(c.Key(key), text, size, lineH, weight, theme.C("ink-muted")).SingleLine().MinWidth(0).Margin(top, 0, 0, 0)
	if path == "" || p.env.HasRoute == nil || !p.env.HasRoute(path) {
		return
	}
	t.Role(ui.RoleLink).Cursor(ui.CursorPointer).Focusable()
	if t.Hovered() {
		t.Underline().TextColor(theme.C("ink"))
	}
	if t.Clicked() {
		p.ui.closeOverlay()
		p.env.Nav.Push(path)
	}
}

// songCopy is the song's label copy under (or, narrow, beside) the cover: the
// title, the artist and the album as links out, the marks the bar also shows
// (Explicit, Preview), and the song's own actions: Favourite, Suggest Less and
// its credits. A local broadcaster is headed by the song its stream names on
// air; a music video offers its picture.
func (p *player) songCopy(c *ui.Context, cur *services.QueueItem, large bool) {
	s := p.env.Play.State()
	broadcast := cur.Live && cur.Broadcast
	heading, byline := cur.Name, cur.Artist
	if broadcast && s.OnAir != nil {
		heading, byline = s.OnAir.Title, s.OnAir.Artist
	}
	tk := theme.Current().Type
	ui.Column(c.Key("song-copy")).FillWidth().MinWidth(0).Children(func() {
		style := tk["page"]
		if large {
			style = tk["display"]
		}
		ui.Text(c.Key("title"), heading).FontSize(style.Size).FixedLineHeight(style.LineHeight).FontWeight(int(style.Weight)).
			LetterSpacing(style.Tracking).TextColor(theme.C("ink")).MaxLines(3).Role(ui.RoleHeading).Level(2)
		if cur.Live {
			if broadcast && byline != "" {
				lineText(c.Key("byline"), byline, 16, 22, 500, theme.C("ink-muted")).Margin(8, 0, 0, 0).SingleLine()
			}
			p.liveLines(c, cur, s, broadcast && s.OnAir != nil)
			if broadcast {
				p.broadcasterBlock(c, cur.Name, s.Broadcaster)
			}
		} else {
			if cur.Artist != "" {
				p.outLink(c, "artist", cur.Artist, idPath("/artist/", cur.ArtistID), 16, 22, 500, 8)
			}
			if cur.Album != "" {
				p.outLink(c, "album", cur.Album, idPath("/album/", cur.AlbumID), 13, 18, 400, 2)
			}
			if cur.Explicit || s.Preview {
				ui.Row(c.Key("marks")).Margin(10, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
					if cur.Explicit {
						kit.ExplicitMark(c.Key("explicit"))
					}
					if s.Preview {
						kit.PreviewMark(c.Key("preview"))
					}
				})
			}
		}
		if cur.Video {
			if kit.Button(c.Key("show-video"), i18n.T("Show video"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "clapperboard"}).
				Margin(12, 0, 0, 0).Clicked() {
				p.env.Video.Open()
			}
		}
		if !cur.Live && !cur.Video && (cur.CatalogID != "" || cur.LibraryID != "") {
			ui.Row(c.Key("actions")).Margin(12, 0, 0, -8).Gap(4).AlignItems(ui.Center).Children(func() {
				if p.np.rating != nil {
					p.np.rating(c, cur)
				}
				credits := "/song/" + cur.CatalogID
				if cur.CatalogID != "" && p.env.HasRoute != nil && p.env.HasRoute(credits) {
					if kit.Button(c.Key("credits"), i18n.T("View credits"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "info", Muted: true}).Clicked() {
						p.ui.closeOverlay()
						p.env.Nav.Push(credits)
					}
				}
			})
		}
	})
}

// liveLines is a live station's line: Live, and what is on air (the stream's
// own words, else the schedule), unless the heading already says it.
func (p *player) liveLines(c *ui.Context, cur *services.QueueItem, s services.PlaybackState, onAirInHeading bool) {
	playing := s.State == "playing" || s.State == "loading"
	dot := theme.C("ink-faint")
	if playing {
		dot = theme.C("play")
	}
	ui.Row(c.Key("live")).Margin(8, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c.Key("dot")).Size(8, 8).Radius(4).Background(dot).Role(ui.RoleNone)
		lineText(c.Key("live-text"), i18n.T("Live"), 16, 22, 500, theme.C("ink"))
	})
	if onAirInHeading {
		return
	}
	if s.OnAir != nil {
		line := s.OnAir.Title
		if s.OnAir.Artist != "" {
			line = s.OnAir.Artist + " – " + s.OnAir.Title
		}
		lineText(c.Key("on-air"), line, 13, 18, 400, theme.C("ink-muted")).Margin(4, 0, 0, 0).MaxLines(2)
		return
	}
	if ev := p.onAirProgramme(c, cur.ID); ev != nil {
		until := i18n.FormatClock(time.UnixMilli(ev.EndMs))
		lineText(c.Key("programme"), ev.Title+" · "+i18n.T("until {until}", "until", until), 13, 18, 400, theme.C("ink-muted")).
			Margin(4, 0, 0, 0).MaxLines(2)
	}
}

// onAirProgramme is the programme on air on live station id (nil when the
// station has no schedule): the station's page query, shared with the radio
// pages, and a wake-up when the slot ends.
func (p *player) onAirProgramme(c *ui.Context, id string) *services.StationEvent {
	if id == "" {
		return nil
	}
	svc := services.Service[services.RadioService](p.env.Core)
	r := store.Use(p.env.Queries, store.Key{"radio", "station", id}, func(ctx context.Context) (services.StationDetail, error) {
		return svc.Station(ctx, id)
	}, store.Options{Stale: 5 * time.Minute})
	if !r.HasData {
		return nil
	}
	now := p.now().UnixMilli()
	for i := range r.Data.Schedule {
		if e := &r.Data.Schedule[i]; e.StartMs <= now && now < e.EndMs {
			c.After(time.Duration(max(e.EndMs-now, 1000)) * time.Millisecond)
			return e
		}
	}
	return nil
}

// broadcasterBlock is what the view says about a local broadcaster under the
// song on air: the station's name with its genre and place, its description
// and a link to its website.
func (p *player) broadcasterBlock(c *ui.Context, name string, info *services.BroadcasterInfo) {
	parts := []string{name}
	if info != nil {
		for _, s := range []string{info.Genre, info.Location} {
			if s != "" {
				parts = append(parts, s)
			}
		}
	}
	muted := theme.C("ink-muted")
	ui.Column(c.Key("broadcaster")).Margin(16, 0, 0, 0).MinWidth(0).Children(func() {
		ui.Row(c.Key("on-air-line")).MinWidth(0).Gap(0).Children(func() {
			lineText(c.Key("on-air-label"), i18n.T("On air"), 13, 18, 500, theme.C("ink"))
			lineText(c.Key("on-air-rest"), " · "+strings.Join(parts, " · "), 13, 18, 400, muted).SingleLine().MinWidth(0).Shrink(1)
		})
		if info == nil {
			return
		}
		if info.Description != "" {
			lineText(c.Key("description"), info.Description, 13, 18, 400, muted).MaxLines(3).Margin(4, 0, 0, 0)
		}
		if info.Website != "" {
			if kit.Button(c.Key("website"), i18n.T("Open website"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Margin(12, 0, 0, 0).Clicked() {
				c.Services().OpenURL(info.Website)
			}
		}
	})
}

// queuePane is Up next: the queue panel's list with its foot strip (the queue
// summary, leaving closes the view, and the queue tools), when there is one.
func (p *player) queuePane(c *ui.Context) {
	if p.np.queueView != nil {
		p.np.queueView(c, func() { p.ui.closeOverlay() })
	}
}

// lyricsPane is the Lyrics pane: the sheet over a size container (so the first
// line starts at 14% of the sheet's height however tall the window), and the
// tools strip below it.
func (p *player) lyricsPane(c *ui.Context, songID string) {
	if p.np.sheet == nil || p.np.sheet.songID != songID {
		// The pane is remounted per song: scroll and focus start again.
		p.np.sheet = newLyricsSheet(songID)
	}
	ui.Column(c.Key("lyrics-box:" + songID)).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
		p.lyricsBody(c, songID)
	})
	p.lyricsTools(c, songID)
}

// lyricsBody is the sheet, or what stands in for it while the lyrics load,
// failed or do not exist.
func (p *player) lyricsBody(c *ui.Context, songID string) {
	r := p.lyricsFor(songID)
	switch {
	case !r.HasData && r.Err == nil:
		ui.Column(c.Key("loading")).FillWidth().Gap(20).Padding(60, 12, 0).Role(ui.RoleStatus).Label(i18n.T("Loading lyrics")).Children(func() {
			for i, w := range []float32{72, 56, 64, 40, 60} {
				ui.Box(c.Key("bar" + itoa(i))).FillWidth().Children(func() {
					kit.Skeleton(c, 0, 24).WidthPercent(w)
				})
			}
		})
	case lyricsMissing(r):
		p.noLyrics(c)
	case !r.HasData:
		kit.ErrorBlock(c, i18n.T("Couldn't load the lyrics"), r.Err, kit.ErrorBlockOpts{
			OnRetry: func() { p.retryLyrics(songID) }, Retrying: r.Fetching,
		}).Margin(96, 0, 0, 0)
	default:
		p.lyricsScroller(c, p.np.sheet, &r.Data, sheetIn{look: sheetLook(), clickable: true})
	}
}

// noLyrics is the empty card of a song none of the lyrics sources has.
func (p *player) noLyrics(c *ui.Context) {
	var action *kit.EmptyAction
	if p.env.HasRoute != nil && p.env.HasRoute("/settings") {
		action = &kit.EmptyAction{Label: i18n.T("Lyrics sources"), Run: func() {
			p.ui.closeOverlay()
			p.env.Nav.Push("/settings?section=playback")
		}}
	}
	kit.EmptyState(c.Key("no-lyrics"), i18n.T("No lyrics for this song"), i18n.T("None of the lyrics sources in Settings has lyrics for it."),
		kit.EmptyOpts{Action: action}).MaxWidth(448).Margin(60, 0, 0, 0)
}

// languageName is a language tag's name in the UI's language ("en-US" →
// "English"), the tag itself when it has none.
func languageName(tag string) string {
	t, err := language.Parse(tag)
	if err != nil {
		return tag
	}
	base, _ := t.Base()
	name := display.Languages(language.Make(i18n.Locale())).Name(language.Make(base.String()))
	if name == "" {
		return tag
	}
	return name
}

// lyricsTools is the lyric sheet's own controls, in a strip at its foot as
// full screen keeps them: translation and pronunciation at the start (only
// when the lyrics have them for this song; each adds its line under every
// sung line, and the choice is kept for every song), then, for synced lyrics,
// the timing nudge and full screen at the end.
func (p *player) lyricsTools(c *ui.Context, songID string) {
	r := p.lyricsFor(songID)
	if !r.HasData || len(r.Data.Lines) == 0 {
		return
	}
	l := &r.Data
	hasTr, hasPr := lyricsync.Extras(l)
	synced := lyricsync.IsSynced(l) && len(l.Lines) > 0
	if !hasTr && !hasPr && !synced {
		return
	}
	prefs := p.prefsFor()
	cur := prefs.get()
	ui.Row(c.Key("lyrics-tools")).FillWidth().Shrink(0).Gap(12).PaddingStart(2).Padding(12, 0, 0, 2).AlignItems(ui.Center).Children(func() {
		if hasTr || hasPr {
			ui.Row(c.Key("toggles")).Gap(4).AlignItems(ui.Center).Children(func() {
				if hasTr {
					on := cur.Translation
					if kit.Button(c.Key("translation"), i18n.T("Translation"), kit.ButtonOpts{
						Variant: kit.Ghost, Size: kit.SizeSM, Icon: "languages", Pressed: &on,
						Tip: i18n.T("Translation into {language}", "language", languageName(l.TranslationLanguage)),
					}).Clicked() {
						cur.Translation = !cur.Translation
						prefs.set(cur)
					}
				}
				if hasPr {
					on := cur.Pronunciation
					if kit.Button(c.Key("pronunciation"), i18n.T("Pronunciation"), kit.ButtonOpts{
						Variant: kit.Ghost, Size: kit.SizeSM, Icon: "spell-check", Pressed: &on, Tip: i18n.T("The words in Latin letters"),
					}).Clicked() {
						cur.Pronunciation = !cur.Pronunciation
						prefs.set(cur)
					}
				}
			})
		}
		if synced {
			ui.Box(c.Key("grow")).Grow(1)
			ui.Row(c.Key("timing-room")).Gap(4).AlignItems(ui.Center).Children(func() {
				p.lyricsTiming(c, songID)
				if kit.Button(c.Key("room"), i18n.T("Full screen"), kit.ButtonOpts{
					Variant: kit.Ghost, Size: kit.SizeSM, Icon: "maximize-2",
					Tip: i18n.T("Lyrics across the whole screen, large enough to read across a room"),
				}).Bind(&p.np.roomToggle).Clicked() {
					p.ui.openRoom()
				}
			})
		}
	})
}
