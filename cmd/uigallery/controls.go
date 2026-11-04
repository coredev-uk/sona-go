package main

// The kit-controls sections of the gallery. The build tag stays until
// cmd/uigallery (PR 0.3) is on main; the lead removes it at the rebase.

import (
	"errors"
	"net"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlsState is what the sections of this file edit, kept across frames.
var controlsState = struct {
	text, empty, invalid, query, pass string
	combo, plan, lang, empty2         string
	on, off, small                    bool
	vol, seek, gain                   float64
	view                              string
	tab, tabLine, tabSide             int
	progress                          float64
	rating                            int
	inLibrary, pending                bool
	retrying                          bool
	errKind                           int
	noteText                          string
}{
	text: "Radiohead", invalid: "not an email", combo: "France", plan: "pro", lang: "Language 3",
	on: true, small: true, vol: 40, seek: 70, view: "grid", progress: 0.45,
	noteText: "Radiohead's third album, recorded in an Oxfordshire mansion and released in 1997, turned a band known for guitars towards drum machines, mellotrons and a colder, wider sound.\n\nIt was ranked among the greatest albums of its decade by critics and listeners alike, and has outlived most of the records it was compared with at the time.\nA third line in the second paragraph.",
}

func controlsHeading(c *ui.Context, s string) {
	t := theme.Current().Type["heading"]
	ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).TextColor(theme.C("ink")).Margin(20, 0, 4)
}

func controlsNote(c *ui.Context, s string) {
	t := theme.Current().Type["secondary"]
	ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink-muted"))
}

func controlsRow(c *ui.Context, key string, fn func()) {
	ui.Row(c.Key(key)).Gap(12).AlignItems(ui.Center).Wrap().Children(fn)
}

func init() {
	register(section{name: "Controls / Badges and marks", build: func(c *ui.Context, g *gallery) {
		controlsNote(c, "kit.Badge (six variants), ExplicitMark, Tag, PreviewMark, SDMark, Badges, Separator and Skeleton.")
		controlsHeading(c, "Badge")
		controlsRow(c, "badges", func() {
			for i, name := range []string{"Default", "Secondary", "Destructive", "Outline", "Ghost", "Link"} {
				kit.Badge(c.Key(i), name, kit.BadgeOpts{Variant: kit.BadgeVariant(i)})
			}
			kit.Badge(c.Key("i"), "Interactive", kit.BadgeOpts{Variant: kit.BadgeOutline, Icon: "plus", Interactive: true})
		})
		controlsHeading(c, "Marks")
		controlsRow(c, "marks", func() {
			ui.Text(c, "A song title")
			kit.ExplicitMark(c)
			kit.PreviewMark(c)
			kit.SDMark(c)
		})
		kit.Badges(c, kit.BadgesOpts{Explicit: true, Traits: services.Traits{Atmos: true, Lossless: true, AppleDigitalMaster: true}, Extra: []string{"4K", "HDR"}})
		controlsHeading(c, "Separator and Skeleton")
		kit.Separator(c, false)
		ui.Row(c).Height(24).Gap(12).AlignItems(ui.Center).Margin(8, 0).Children(func() {
			ui.Text(c, "One")
			kit.Separator(c, true)
			ui.Text(c, "Two")
		})
		kit.Skeleton(c, 240, 16)
		kit.Skeleton(c.Key("s2"), 0, 16)
	}})

	register(section{name: "Controls / Input and SearchField", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Input is 36 high with a control-line border that turns ring-coloured on focus; SearchField adds the glass, Clear and Escape.")
		ui.Column(c).Gap(10).Width(360).Children(func() {
			kit.Input(c.Key("a"), &s.text, kit.InputOpts{A11yLabel: "Name"})
			kit.Input(c.Key("b"), &s.empty, kit.InputOpts{A11yLabel: "Placeholder", Placeholder: "Placeholder"})
			kit.Input(c.Key("c"), &s.invalid, kit.InputOpts{A11yLabel: "Invalid", Invalid: true})
			kit.Input(c.Key("d"), &s.pass, kit.InputOpts{A11yLabel: "Password", Password: true, Placeholder: "Password"})
			kit.Input(c.Key("e"), &s.empty2, kit.InputOpts{A11yLabel: "Disabled", Disabled: true, Placeholder: "Disabled"})
			kit.SearchField(c.Key("f"), &s.query, kit.SearchFieldOpts{Placeholder: "Search your library"})
		})
	}})

	register(section{name: "Controls / Combobox", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Combobox: type to filter, Down shows all, Enter or a click chooses, Escape closes.")
		kit.Combobox(c, &s.combo, []string{"France", "Frankfurt", "Freetown", "Germany", "Spain", "Sweden"}, kit.ComboboxOpts{A11yLabel: "Country", Width: 260})
	}})

	register(section{name: "Controls / Switch", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Switch: default and small, ink when on, the thumb slides over 140 ms (not under reduced motion).")
		controlsRow(c, "switches", func() {
			kit.Switch(c.Key("1"), &s.on, kit.SwitchOpts{A11yLabel: "On"})
			kit.Switch(c.Key("2"), &s.off, kit.SwitchOpts{A11yLabel: "Off"})
			kit.Switch(c.Key("3"), &s.small, kit.SwitchOpts{A11yLabel: "Small", Small: true})
			kit.Switch(c.Key("4"), &s.on, kit.SwitchOpts{A11yLabel: "Disabled", Disabled: true})
		})
	}})

	register(section{name: "Controls / Select", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Select: groups with labels and separators, a check on the chosen row, a long list scrolls.")
		plans := []kit.SelectItem{
			kit.SelectLabel("Plans"), kit.SelectOption("free", "Free"), kit.SelectOption("pro", "Pro"), kit.SelectSeparator(),
			kit.SelectLabel("Teams"), kit.SelectOption("team", "Team"),
			{Value: "ent", Label: "Enterprise", Disabled: true},
		}
		var long []kit.SelectItem
		for i := 1; i <= 40; i++ {
			v := "Language " + string(rune('0'+i/10)) + string(rune('0'+i%10))
			long = append(long, kit.SelectOption(v, v))
		}
		controlsRow(c, "selects", func() {
			kit.Select(c.Key("1"), &s.plan, plans, kit.SelectOpts{A11yLabel: "Plan"})
			kit.Select(c.Key("2"), &s.plan, plans, kit.SelectOpts{A11yLabel: "Plan, small", Small: true})
			kit.Select(c.Key("3"), &s.lang, long, kit.SelectOpts{A11yLabel: "Language"})
			kit.Select(c.Key("4"), &s.empty2, plans, kit.SelectOpts{A11yLabel: "Placeholder", Placeholder: "Pick one"})
		})
	}})

	register(section{name: "Controls / Segmented and Tabs", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Segmented (two to four exclusive choices) and kit.Tabs (default, line and vertical).")
		kit.Segmented(c, &s.view, []kit.SegmentOption[string]{{Value: "list", Label: "List"}, {Value: "grid", Label: "Grid"}, {Value: "table", Label: "Table"}}, "View")
		controlsHeading(c, "Tabs")
		labels := []string{"General", "Playback", "Advanced"}
		kit.Tabs(c.Key("t1"), &s.tab, labels, kit.TabsOpts{A11yLabel: "Settings"})
		kit.Tabs(c.Key("t2"), &s.tabLine, labels, kit.TabsOpts{Line: true, A11yLabel: "Settings, line"})
		ui.Row(c.Key("side")).Gap(16).Children(func() {
			kit.Tabs(c.Key("t3"), &s.tabSide, labels, kit.TabsOpts{Line: true, Vertical: true, A11yLabel: "Settings, vertical"})
			kit.TabPanel(c, labels[s.tabSide])
			ui.Text(c, "The page of "+labels[s.tabSide])
		})
	}})

	register(section{name: "Controls / Slider and ProgressBar", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Slider: 4px track, 12px ink thumb always drawn, a ring while keyboard-focused. kit.ProgressBar: a download's bar.")
		ui.Column(c).Gap(14).Width(360).Children(func() {
			kit.Slider(c.Key("1"), &s.vol, 0, 100, kit.SliderOpts{A11yLabel: "Volume", ValueText: "volume"})
			kit.Slider(c.Key("2"), &s.seek, 0, 100, kit.SliderOpts{A11yLabel: "Seek", Range: theme.C("amber")})
			kit.Slider(c.Key("3"), &s.vol, 0, 100, kit.SliderOpts{A11yLabel: "Disabled", Disabled: true})
			kit.Slider(c.Key("4"), &s.gain, -12, 12, kit.SliderOpts{A11yLabel: "Stepped", Step: 1})
			kit.ProgressBar(c.Key("5"), s.progress, kit.ProgressOpts{A11yLabel: "Download"})
			kit.ProgressBar(c.Key("6"), -1, kit.ProgressOpts{A11yLabel: "Starting", Height: 6})
		})
		ui.Slider(c.Key("p"), &s.progress, 0, 1).Label("Progress").Width(200).Margin(8, 0)
	}})

	register(section{name: "Controls / Tooltip", build: func(c *ui.Context, g *gallery) {
		controlsNote(c, "kit.Tooltip: an ink bubble with an arrow after the pointer rests (or the keyboard focus arrives); Escape hides it first.")
		controlsRow(c, "tips", func() {
			kit.IconButton(c.Key("1"), "ellipsis", "More actions", kit.ButtonOpts{Variant: kit.Outline})
			kit.Tooltip(c, kit.Button(c.Key("2"), "Hover me", kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}), "A tip says what the button does")
		})
	}})

	register(section{name: "Controls / Rating, Library and Play buttons", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.RatingButtons, LibraryButton, PlayButtons and PlayButton report what was asked for; the caller owns the state.")
		controlsRow(c, "rating", func() {
			switch kit.RatingButtons(c, kit.RatingOpts{Name: "OK Computer", Rating: s.rating, Pending: s.pending}) {
			case kit.RateFavorite:
				s.rating = 1
			case kit.RateUnfavorite, kit.RateUndoSuggestLess:
				s.rating = 0
			case kit.RateSuggestLess:
				s.rating = -1
			}
		})
		controlsRow(c, "library", func() {
			switch kit.LibraryButton(c, kit.LibraryOpts{Name: "OK Computer", InLibrary: s.inLibrary, Pending: s.pending}) {
			case kit.LibraryAdd:
				s.inLibrary = true
			case kit.LibraryRemove:
				s.inLibrary = false
			}
		})
		controlsRow(c, "play", func() {
			ui.Row(c.Key("on")).Gap(8).AlignItems(ui.Center).Children(func() {
				kit.PlayButtons(c, kit.PlayButtonsOpts{Label: "OK Computer", Shuffle: true})
			})
			ui.Row(c.Key("dis")).Gap(8).AlignItems(ui.Center).Children(func() {
				kit.PlayButtons(c, kit.PlayButtonsOpts{Label: "OK Computer", Shuffle: true, Disabled: true})
			})
			kit.PlayButton(c, "Play OK Computer", 48)
		})
		ui.Checkbox(c, &s.pending, "Pending").Margin(8, 0)
	}})

	register(section{name: "Controls / Empty and error states", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.EmptyState (box and covers), ErrorBlock, DetailSkeleton and DetailError.")
		ui.Column(c).Gap(16).Children(func() {
			kit.EmptyState(c.Key("1"), "No albums yet", "Add music from Apple Music and it appears here.", kit.EmptyOpts{Action: &kit.EmptyAction{Label: "Browse", Run: func() {}}})
			kit.EmptyState(c.Key("2"), "No albums yet", "Add music from Apple Music and it appears here.", kit.EmptyOpts{Frame: kit.FrameCovers})
			kit.ErrorBlock(c.Key("3"), "Couldn't load this album", &net.DNSError{Err: "no such host"}, kit.ErrorBlockOpts{
				OnRetry: func() { s.retrying = !s.retrying }, Retrying: s.retrying,
			})
			kit.DetailError(c.Key("4"), kit.NounAlbum, errors.New("boom"), kit.DetailErrorOpts{Retry: func() {}})
			kit.DetailSkeleton(c.Key("5"), false)
		})
	}})

	register(section{name: "Controls / Headers", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.PageHeader and kit.CollectionHeader (cover, title, byline, meta, badges, description, actions).")
		kit.PageHeader(c, "Albums", "128 albums, 42 hours", func() {
			kit.Button(c, "Sort", kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM})
		})
		artists := []services.Artist{{ID: "1", Name: "Radiohead"}}
		kit.CollectionHeader(c, kit.CollectionHeaderOpts{
			Title: "OK Computer", Art: "/art/k1", Loader: g.art,
			Byline: func() { kit.ArtistLinks(c, artists, "Radiohead", ui.Color{}) },
			Meta:   "Album · 1997 · 12 songs, 53 min",
			Badges: func() { kit.Badges(c, kit.BadgesOpts{Explicit: true, Traits: services.Traits{Lossless: true}}) },
			Description: "Radiohead's third album, recorded in an Oxfordshire mansion and released in 1997, turned a band known for guitars " +
				"towards drum machines, mellotrons and a colder, wider sound.",
			Actions: func() {
				kit.PlayButtons(c, kit.PlayButtonsOpts{Label: "OK Computer", Shuffle: true})
				kit.LibraryButton(c, kit.LibraryOpts{Name: "OK Computer", InLibrary: s.inLibrary})
				kit.RatingButtons(c, kit.RatingOpts{Name: "OK Computer", Rating: s.rating})
			},
		})
	}})

	register(section{name: "Controls / Notes and ArtistLinks", build: func(c *ui.Context, g *gallery) {
		s := &controlsState
		controlsNote(c, "kit.Notes clamps to four lines and offers Read more only when the text runs longer; kit.ArtistLinks joins credits as \"A, B & C\".")
		kit.Notes(c, s.noteText, 0)
		controlsHeading(c, "ArtistLinks")
		all := []services.Artist{{ID: "1", Name: "Daft Punk"}, {ID: "2", Name: "Pharrell Williams"}, {ID: "3", Name: "Nile Rodgers"}}
		kit.ArtistLinks(c.Key("3"), all, "Daft Punk", ui.Color{})
		kit.ArtistLinks(c.Key("2"), all[:2], "Daft Punk", ui.Color{})
		kit.ArtistLinks(c.Key("1"), nil, "Daft Punk, Pharrell Williams & Nile Rodgers", ui.Color{})
	}})
}
