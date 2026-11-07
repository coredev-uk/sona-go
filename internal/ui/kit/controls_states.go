package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// EmptyAction is the one action of an EmptyState or a DetailError: a labelled
// button that runs Run (navigate somewhere, open the account dialog).
type EmptyAction struct {
	Label string
	Run   func()
}

// EmptyFrame is how an EmptyState marks where the data will appear.
type EmptyFrame int

// The frames.
const (
	// FrameBox is a dashed box (lists, panels).
	FrameBox EmptyFrame = iota
	// FrameCovers is one row of empty cover slots above the text, for pages
	// whose content is a grid of artwork.
	FrameCovers
)

// EmptyOpts are the options of EmptyState.
type EmptyOpts struct {
	// Action is at most one: an outline small button.
	Action *EmptyAction
	Frame  EmptyFrame
}

// EmptyState is the frame for a block with nothing in it yet: where the data
// will appear, one sentence-case title, one sentence, at most one action
// (shared/components/EmptyState.tsx).
func EmptyState(c *ui.Context, title, text string, o EmptyOpts) ui.Element {
	e := ui.Column(c).AlignItems(ui.Start).Gap(4).FillWidth()
	body := func() {
		controlsText(c, title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
		controlsText(c, text, "body").TextColor(theme.C("ink-muted")).MaxWidth(480)
		if o.Action != nil && o.Action.Run != nil {
			if Button(c.Key("action"), o.Action.Label, ButtonOpts{Variant: Outline, Size: SizeSM}).Margin(16, 0, 0, 0).Clicked() {
				o.Action.Run()
			}
		}
	}
	if o.Frame == FrameCovers {
		e.Children(func() {
			emptyCoverSlots(c, controlsWatchWidth(c, e))
			body()
		})
		return e
	}
	e.Padding(28, 24).Radius(controlsRadius("lg")).Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed)
	e.Children(body)
	return e
}

// emptyCoverSlots is one row of dashed square slots at the grid's real tile
// size (160 at least, 24 apart), as many as fit in width. Before the first
// frame has measured the page it is 160 high.
func emptyCoverSlots(c *ui.Context, width float32) {
	const minTile, gap = 160, 24
	n := max(1, int((width+gap)/(minTile+gap)))
	tile := float32(minTile)
	if width > 0 {
		tile = (width - float32(n-1)*gap) / float32(n)
	}
	line := theme.C("control-line")
	r := controlsRadius("sm")
	ui.Box(c).FillWidth().Height(tile).Margin(0, 0, 20, 0).Shrink(0).Role(ui.RoleNone).Draw(func(p *ui.Painter, b ui.Rect) {
		for i := 0; i < n; i++ {
			p.StrokeDashed(ui.Rect{X: b.X + float32(i)*(tile+gap), Y: b.Y, W: tile, H: tile}, line, r, 1)
		}
	})
}

// errorDetail is Go's failure, worded from its code in the language shown;
// empty for no error. Go's own text never reaches the screen.
func errorDetail(err error) string {
	if err == nil {
		return ""
	}
	return i18n.ErrorText(services.CodeOf(err))
}

// ErrorBlockOpts are the options of ErrorBlock.
type ErrorBlockOpts struct {
	// OnRetry shows a "Try again" text button that calls it; nil shows none.
	OnRetry func()
	// Retrying disables that button and words it "Retrying…".
	Retrying bool
}

// ErrorBlock is an error inside the block that failed: a dashed box with a
// danger CircleAlert, what failed in the product's words (title), Go's
// reason worded from err's code, and a Try again button. --danger only on
// the icon. The title and the reason are an alert: assistive technology
// reads them out when the block appears, and again if they change.
// (shared/components/ErrorBlock.tsx)
func ErrorBlock(c *ui.Context, title string, err error, o ErrorBlockOpts) ui.Element {
	detail := errorDetail(err)
	e := ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Padding(16, 20).Radius(controlsRadius("lg")).
		Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed)
	e.Children(func() {
		ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Role(ui.RoleAlert).Label(title).Children(func() {
			controlsText(c, title, "label").TextColor(theme.C("ink"))
			if detail != "" {
				controlsText(c, detail, "secondary").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
			}
		})
		if o.OnRetry != nil {
			label := i18n.T("Try again")
			if o.Retrying {
				label = i18n.T("Retrying…")
			}
			if Button(c.Key("retry"), label, ButtonOpts{Variant: Ghost, Size: SizeSM, Disabled: o.Retrying}).Margin(-4, 0, -4, 0).Clicked() {
				o.OnRetry()
			}
		}
	})
	return e
}

// DetailSkeleton is the collection header and track rows at their real sizes
// while a detail page loads: flat --hover bars, a 200px cover (a circle for
// an artist), action discs and 8 rows. It is a labelled "Loading" status
// for assistive technology. (shared/components/DetailStates.tsx)
func DetailSkeleton(c *ui.Context, round bool) ui.Element {
	hover := theme.C("hover")
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(controlsRadius("sm")).Background(hover).Shrink(0)
	}
	e := ui.Column(c).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading"))
	e.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.End).Gap(28).Padding(4, 0, 28, 0).Margin(0, 0, 24, 0).Children(func() {
			radius := controlsRadius("sm")
			if round {
				radius = controlsRadius("full")
			}
			ui.Box(c).Size(200, 200).Radius(radius).Background(hover).Shrink(0)
			ui.Column(c).Grow(1).Padding(0, 0, 4, 0).Children(func() {
				ui.Box(c).Height(32).WidthPercent(40).Radius(controlsRadius("sm")).Background(hover)
				ui.Box(c).Height(16).WidthPercent(25).Radius(controlsRadius("sm")).Background(hover).Margin(12, 0, 0, 0)
				ui.Box(c).Height(12).WidthPercent(33).Radius(controlsRadius("sm")).Background(hover).Margin(8, 0, 0, 0)
				ui.Row(c).Gap(8).Margin(24, 0, 0, 0).Children(func() {
					ui.Box(c).Size(40, 40).Radius(controlsRadius("full")).Background(hover)
					ui.Box(c).Size(96, 32).Radius(controlsRadius("md")).Background(hover)
				})
			})
		})
		for i := 0; i < 8; i++ {
			ui.Row(c.Key(i)).FillWidth().Height(44).PaddingX(8).Gap(16).AlignItems(ui.Center).Children(func() {
				bar(20, 12)
				ui.Box(c).Height(14).WidthPercent(float32(30 + (i*17)%30)).Radius(controlsRadius("sm")).Background(hover)
			})
		}
	})
	return e
}

// DetailNoun is what a detail page shows, in its error words.
type DetailNoun int

// The nouns.
const (
	NounAlbum DetailNoun = iota
	NounArtist
	NounPlaylist
	NounSong
	NounMusicVideo
)

// DetailErrorOpts are the options of DetailError.
type DetailErrorOpts struct {
	// Retry and Retrying are those of ErrorBlock, for failures that are
	// neither "not found" nor "not signed in".
	Retry    func()
	Retrying bool
	// Back is the link for "not found": a library page (Albums, Artists,
	// Playlists, Songs) or Search.
	Back EmptyAction
	// SignIn opens the account dialog (the "not signed in" button).
	SignIn func()
}

// DetailError is a detail page that failed: "This {noun} isn't available"
// with the explanation and a back link for a not-found error, "Sign in to
// see this" with a Sign in button for a signed-out one, otherwise an
// ErrorBlock "Couldn't load this {noun}" with the reason and Try again.
// (shared/components/DetailStates.tsx)
func DetailError(c *ui.Context, noun DetailNoun, err error, o DetailErrorOpts) ui.Element {
	switch services.CodeOf(err) {
	case services.CodeNotFound:
		var title string
		switch noun {
		case NounAlbum:
			title = i18n.T("This album isn't available")
		case NounArtist:
			title = i18n.T("This artist isn't available")
		case NounPlaylist:
			title = i18n.T("This playlist isn't available")
		case NounSong:
			title = i18n.T("This song isn't available")
		default:
			title = i18n.T("This music video isn't available")
		}
		back := o.Back
		return EmptyState(c, title,
			i18n.T("It isn't in your library on this computer, and Apple Music didn't return it. It may not be offered in your storefront, or it was removed."),
			EmptyOpts{Action: &back}).Margin(8, 0, 0, 0)
	case services.CodeNotSignedIn:
		var body string
		switch noun {
		case NounAlbum:
			body = i18n.T("Apple Music needs a signed-in account to show this album.")
		case NounArtist:
			body = i18n.T("Apple Music needs a signed-in account to show this artist.")
		case NounPlaylist:
			body = i18n.T("Apple Music needs a signed-in account to show this playlist.")
		case NounSong:
			body = i18n.T("Apple Music needs a signed-in account to show this song.")
		default:
			body = i18n.T("Apple Music needs a signed-in account to show this music video.")
		}
		return EmptyState(c, i18n.T("Sign in to see this"), body,
			EmptyOpts{Action: &EmptyAction{Label: i18n.T("Sign in"), Run: o.SignIn}}).Margin(8, 0, 0, 0)
	}
	var title string
	switch noun {
	case NounAlbum:
		title = i18n.T("Couldn't load this album")
	case NounArtist:
		title = i18n.T("Couldn't load this artist")
	case NounPlaylist:
		title = i18n.T("Couldn't load this playlist")
	case NounSong:
		title = i18n.T("Couldn't load this song")
	default:
		title = i18n.T("Couldn't load this music video")
	}
	return ErrorBlock(c, title, err, ErrorBlockOpts{OnRetry: o.Retry, Retrying: o.Retrying}).Margin(8, 0, 0, 0)
}

// PageHeader is the title row of a page: the title at 24/30 (one line, cut
// with an ellipsis), an optional subline under it (one line about the data
// or the object, never about the interface) and the page's actions at the
// end, bottom-aligned (shared/components/PageHeader.tsx). actions builds
// into a row; nil for none.
func PageHeader(c *ui.Context, title, subline string, actions func()) ui.Element {
	return pageHeader(c, title, subline, false, actions)
}

// PageHeaderCounted is PageHeader for a page whose subline counts its list:
// the line is blank until the count is known (a sort or filter not read yet
// loads its list), and its place is kept, so that the actions beside it do
// not move when the count arrives.
func PageHeaderCounted(c *ui.Context, title, subline string, actions func()) ui.Element {
	return pageHeader(c, title, subline, true, actions)
}

func pageHeader(c *ui.Context, title, subline string, keepSubline bool, actions func()) ui.Element {
	e := ui.Row(c).FillWidth().AlignItems(ui.End).Justify(ui.SpaceBetween).Gap(24).Margin(0, 0, 24, 0).Shrink(0)
	e.Children(func() {
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			head := controlsText(c, title, "title").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
			if title == "" {
				head.Role(ui.RoleNone) // a page still loading has no title to read
			}
			if subline != "" {
				controlsText(c, subline, "secondary").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
			} else if keepSubline {
				ui.Box(c).Height(4 + theme.Current().Type["secondary"].LineHeight).Shrink(0)
			}
		})
		if actions != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(actions)
		}
	})
	return e
}

// CollectionHeaderOpts are the options of CollectionHeader.
type CollectionHeaderOpts struct {
	Title string
	// Art is the cover's art URL, loaded by Loader; Round is an artist's
	// circle, Video a music video's 16:9 frame.
	Art    string
	Loader *ArtLoader
	Round  bool
	Video  bool
	// Cover replaces the still cover (a motion cover where motion is
	// allowed): it builds a picture size DIPs wide.
	Cover func(c *ui.Context, size float32)
	// Wash is the cover's backdrop behind the header: the header sits inside
	// it (an Absolute child would be drawn over the title and swallow the
	// buttons' clicks), and it bleeds to the content edges with it.
	Wash *BackdropOpts
	// Byline is the line under the title (artist links, a curator).
	Byline func()
	// Meta is the catalogue line: "Album · 2008 · 10 songs, 42 min".
	Meta string
	// Badges are the explicit and audio-quality labels under it.
	Badges func()
	// Description is clamped to two lines.
	Description string
	// Status is a state of the collection above the actions (a release
	// countdown).
	Status func()
	// Actions builds into a wrapping row.
	Actions func()
}

// CollectionHeader is the album, playlist, artist and station header: the
// cover (200, or 136 under 960 DIPs of window), the title at 32/36, the
// byline, catalogue line, badges, description, status and actions, on a
// wash of the cover's colour from Backdrop. It bleeds to the content edges
// by the page's gutter (32, 24 when narrow).
// (shared/components/CollectionHeader.tsx)
func CollectionHeader(c *ui.Context, o CollectionHeaderOpts) ui.Element {
	tk := theme.Current()
	ww, _ := c.Size()
	narrow := ww < 960
	gutter := tk.Metric["gutter"]
	if narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	art := float32(200)
	if narrow {
		art = 136
	}
	if o.Video {
		art = 356
		if narrow {
			art = 240
		}
	}
	body := func() {
		ui.Row(c.Key("row")).FillWidth().AlignItems(ui.End).Gap(28).Children(func() {
			switch {
			case o.Cover != nil:
				o.Cover(c, art)
			default:
				ao := ArtOpts{Round: o.Round, Edge: true}
				if o.Video {
					ao.Aspect = 16.0 / 9
				}
				Artwork(c, o.Loader, o.Art, art, ao)
			}
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(0, 0, 4, 0).Children(func() {
				d := tk.Type["display"]
				ui.Text(c, o.Title).FontSize(d.Size).FixedLineHeight(d.LineHeight).FontWeight(int(d.Weight)).LetterSpacing(d.Tracking).
					TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(1)
				if o.Byline != nil {
					ui.Row(c.Key("byline")).Margin(6, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).FontSize(tk.Type["heading"].Size).
						FontWeight(500).Children(o.Byline)
				}
				if o.Meta != "" {
					controlsText(c, o.Meta, "meta").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
				}
				if o.Badges != nil {
					ui.Row(c.Key("badges")).Margin(8, 0, 0, 0).Children(o.Badges)
				}
				if o.Description != "" {
					controlsText(c, o.Description, "secondary").TextColor(theme.C("ink-muted")).MaxLines(2).MaxWidth(540).Margin(12, 0, 0, 0)
				}
				if o.Status != nil {
					ui.Row(c.Key("status")).Margin(16, 0, 0, 0).Children(o.Status)
				}
				if o.Actions != nil {
					ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(o.Actions)
				}
			})
		})
	}
	if o.Wash == nil {
		return ui.Column(c).AlignSelf(ui.Stretch).Padding(32, gutter, 28, gutter).Margin(-28, -gutter, 24, -gutter).Shrink(0).Children(body)
	}
	return Backdrop(c.Key("wash"), o.Loader, *o.Wash, func() {
		ui.Column(c).FillWidth().Padding(32, gutter, 28, gutter).Children(body)
	}).AlignSelf(ui.Stretch).Shrink(0).Margin(-28, -gutter, 24, -gutter)
}

// Notes shows editorial notes and biographies at a reading measure: plain
// text split into paragraphs on blank lines (line breaks kept), clamped to
// lines lines (4 when 0), with a "Read more" button that appears only when
// the text runs longer and toggles to "Show less".
// (shared/components/Notes.tsx)
func Notes(c *ui.Context, text string, lines int) ui.Element {
	if lines <= 0 {
		lines = 4
	}
	e := ui.Column(c).MaxWidth(560).Shrink(1)
	open := ui.Local(e, "open", func() bool { return false })
	width := ui.Local(e, "measured", func() float32 { return 0 })
	*width = controlsWatchWidth(c, e)
	paras := notesParagraphs(text)
	tk := theme.Current().Type["body"]
	counts := make([]int, len(paras))
	total := 0
	for i, p := range paras {
		counts[i] = notesLines(c, p, *width, tk)
		total += counts[i]
	}
	overflows := total > lines
	e.Children(func() {
		left := lines
		body := ui.Column(c.Key("text")).Role(ui.RoleGroup) // a group: the toggle points at it
		body.Children(func() {
			for i, p := range paras {
				if !*open && left <= 0 {
					break
				}
				t := controlsText(c.Key(i), p, "body").TextColor(theme.C("ink"))
				if i > 0 {
					t.Margin(12, 0, 0, 0)
				}
				if !*open {
					t.MaxLines(min(counts[i], left))
					left -= counts[i]
				}
			}
		})
		if overflows || *open {
			label := i18n.T("Read more")
			if *open {
				label = i18n.T("Show less")
			}
			toggle := ui.ButtonBase(c.Key("toggle")).AlignSelf(ui.Start).Margin(6, 0, 0, 0).Height(20).Radius(controlsRadius("sm")).Expanded(*open).Controls(body).Label(label)
			toggle.Children(func() {
				t := controlsText(c, label, "secondary").FontWeight(500).TextColor(theme.C("ink-muted")).SingleLine()
				if toggle.Hovered() {
					t.TextColor(theme.C("ink")).Underline()
				}
			})
			if toggle.Clicked() {
				*open = !*open
			}
		}
	})
	return e
}

// notesParagraphs splits text on runs of blank lines.
func notesParagraphs(text string) []string {
	var out []string
	var cur []byte
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	nl := 0
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == '\n' {
			nl++
			continue
		}
		if nl >= 2 {
			flush()
		} else if nl == 1 {
			cur = append(cur, '\n')
		}
		nl = 0
		cur = append(cur, ch)
	}
	flush()
	return out
}

// notesLines is how many lines a paragraph takes at width.
func notesLines(c *ui.Context, p string, width float32, t theme.TypeStyle) int {
	span := ui.Span{Text: p, Font: theme.FontFamily, Size: t.Size, Weight: int(t.Weight)}
	_, h := c.MeasureText(width, span)
	_, one := c.MeasureText(0, ui.Span{Text: "Ag", Font: theme.FontFamily, Size: t.Size, Weight: int(t.Weight)})
	if one <= 0 {
		return 1
	}
	return max(1, int(h/one+0.5))
}

// ArtistLinksOpts are ArtistLinks' options.
type ArtistLinksOpts struct {
	// HasRoute says whether a registered page matches a path (Env.HasRoute);
	// nil links every artist.
	HasRoute func(path string) bool
}

// ArtistLinks is every credited artist as a link to their page, "A, B & C",
// in the colour given (--ink when zero). Without artists it shows fallback
// (Apple's combined artist name) as plain text. Links push "/artist/{id}"
// into the page's router; with ArtistLinksOpts.HasRoute a name is a link only
// where that page exists, else plain text.
// (shared/components/ArtistLinks.tsx)
func ArtistLinks(c *ui.Context, artists []services.Artist, fallback string, color ui.Color, opts ...ArtistLinksOpts) ui.Element {
	var o ArtistLinksOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	if color == (ui.Color{}) {
		color = theme.C("ink")
	}
	if len(artists) == 0 {
		return ui.Text(c, fallback).TextColor(color).SingleLine()
	}
	return ui.RichText(c).Role(ui.RoleNone).Children(func() {
		for i, a := range artists {
			if i > 0 {
				sep := ", "
				if i == len(artists)-1 {
					sep = " & "
				}
				ui.Text(c.Key(sep+a.ID), sep)
			}
			if o.HasRoute != nil && !o.HasRoute("/artist/"+a.ID) {
				ui.Text(c.Key(a.ID), a.Name).TextColor(color)
				continue
			}
			ui.Link(c.Key(a.ID), a.Name, "/artist/"+a.ID).TextColor(color)
		}
	})
}
