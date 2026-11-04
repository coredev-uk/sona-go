package kit

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

func TestGoldenButtons(t *testing.T) {
	controlsSheet(t, "buttons", 560, 360, func(c *ui.Context) {
		row := func(key string, f func()) {
			ui.Row(c.Key(key)).Gap(10).AlignItems(ui.Center).Children(f)
		}
		variants := []struct {
			n string
			v ButtonVariant
		}{{"Default", Default}, {"Secondary", Secondary}, {"Outline", Outline}, {"Ghost", Ghost}, {"Destructive", Destructive}, {"Link", Link}}
		row("variants", func() {
			for _, v := range variants {
				Button(c.Key(v.n), v.n, ButtonOpts{Variant: v.v})
			}
		})
		row("sizes", func() {
			Button(c.Key("xs"), "XS", ButtonOpts{Size: SizeXS, Variant: Outline})
			Button(c.Key("sm"), "Small", ButtonOpts{Size: SizeSM, Variant: Outline})
			Button(c.Key("md"), "Default", ButtonOpts{Variant: Outline})
			Button(c.Key("lg"), "Large", ButtonOpts{Size: SizeLG, Variant: Outline})
			Button(c.Key("dis"), "Disabled", ButtonOpts{Variant: Outline, Disabled: true})
		})
		row("icons", func() {
			for _, s := range []ButtonSize{SizeXS, SizeSM, SizeMD, SizeLG} {
				IconButton(c.Key(s), "ellipsis", "More", ButtonOpts{Variant: Outline, Size: s})
			}
			Button(c.Key("lab"), "Add", ButtonOpts{Variant: Outline, Size: SizeSM, Icon: "plus"})
			IconButton(c.Key("play"), "play", "Play", ButtonOpts{Variant: Play, Size: SizeLG, Filled: true})
			IconButton(c.Key("play36"), "play", "Play", ButtonOpts{Variant: Play, Filled: true})
			IconButton(c.Key("play-dis"), "play", "Play", ButtonOpts{Variant: Play, Size: SizeLG, Filled: true, Disabled: true})
			PlayButton(c.Key("disc"), "Play", 48)
		})
		row("play", func() { PlayButtons(c, PlayButtonsOpts{Label: "OK Computer", Shuffle: true}) })
		row("rating", func() {
			for i, o := range []RatingOpts{
				{Name: "OK Computer"},
				{Name: "OK Computer", Rating: 1},
				{Name: "OK Computer", Rating: -1, Compact: true},
				{Name: "OK Computer", Rating: 1, HideSuggestLess: true},
			} {
				ui.Row(c.Key(i)).Gap(2).Children(func() { RatingButtons(c, o) })
			}
		})
		row("library", func() {
			for i, o := range []LibraryOpts{
				{Name: "OK Computer"},
				{Name: "OK Computer", InLibrary: true},
				{Name: "Mix", InLibrary: true, Playlist: true},
				{Name: "OK Computer", AddLabel: "Pre-add"},
			} {
				ui.Row(c.Key(i)).Children(func() { LibraryButton(c, o) })
			}
		})
	}, nil)
}

func TestGoldenBadges(t *testing.T) {
	controlsSheet(t, "badges", 560, 320, func(c *ui.Context) {
		ui.Row(c.Key("badges")).Gap(8).AlignItems(ui.Center).Children(func() {
			for i, v := range []BadgeVariant{BadgeDefault, BadgeSecondary, BadgeDestructive, BadgeOutline, BadgeGhost, BadgeLink} {
				Badge(c.Key(i), []string{"Default", "Secondary", "Destructive", "Outline", "Ghost", "Link"}[v], BadgeOpts{Variant: v})
			}
			Badge(c.Key("icon"), "Interactive", BadgeOpts{Variant: BadgeOutline, Icon: "plus", Interactive: true})
		})
		ui.Row(c.Key("marks")).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Explicit title")
			ExplicitMark(c)
			PreviewMark(c)
			SDMark(c)
		})
		Badges(c, BadgesOpts{Explicit: true, Traits: services.Traits{Atmos: true, Lossless: true, AppleDigitalMaster: true}, Extra: []string{"4K", "HDR"}})
		Separator(c, false)
		ui.Row(c.Key("sep")).Height(24).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "One")
			Separator(c, true)
			ui.Text(c, "Two")
		})
		Skeleton(c, 240, 16)
		Skeleton(c, 0, 16)
	}, nil)
}

// controlsFields is the state the field sheets show.
type controlsFields struct {
	text, empty, invalid, query, pass string
	combo, choice, tall               string
	on, off, small                    bool
	vol, range0                       float64
	seg                               string
	tab, tabLine                      int
}

func newControlsFields() *controlsFields {
	return &controlsFields{
		text: "Radiohead", invalid: "not an email", query: "okc", combo: "Fr", choice: "Pro", tall: "Blue",
		on: true, small: true, vol: 40, range0: 70, seg: "grid",
	}
}

func (f *controlsFields) view(c *ui.Context) {
	plans := []SelectItem{
		SelectLabel("Plans"), SelectOption("Free", "Free"), SelectOption("Pro", "Pro"), SelectSeparator(),
		SelectLabel("Teams"), SelectOption("Team", "Team"),
		{Value: "Ent", Label: "Enterprise", Disabled: true},
	}
	ui.Column(c.Key("a")).Gap(10).Width(360).Children(func() {
		Input(c.Key("in1"), &f.text, InputOpts{A11yLabel: "Name"})
		Input(c.Key("in2"), &f.empty, InputOpts{Placeholder: "Placeholder", A11yLabel: "Empty"})
		Input(c.Key("in3"), &f.invalid, InputOpts{Invalid: true, A11yLabel: "Email"})
		Input(c.Key("in4"), &f.pass, InputOpts{Disabled: true, Placeholder: "Disabled", A11yLabel: "Off"})
		SearchField(c.Key("search"), &f.query, SearchFieldOpts{})
		Combobox(c.Key("combo"), &f.combo, []string{"France", "Frankfurt", "Freetown", "Germany"}, ComboboxOpts{A11yLabel: "Country"})
		ui.Row(c.Key("sel")).Gap(10).Children(func() {
			Select(c.Key("s1"), &f.choice, plans, SelectOpts{A11yLabel: "Plan"})
			Select(c.Key("s2"), &f.tall, plans, SelectOpts{A11yLabel: "Colour", Small: true, Placeholder: "Pick one"})
		})
		ui.Row(c.Key("sw")).Gap(12).AlignItems(ui.Center).Children(func() {
			Switch(c.Key("w1"), &f.on, SwitchOpts{A11yLabel: "On"})
			Switch(c.Key("w2"), &f.off, SwitchOpts{A11yLabel: "Off"})
			Switch(c.Key("w3"), &f.small, SwitchOpts{A11yLabel: "Small on", Small: true})
			Switch(c.Key("w4"), &f.off, SwitchOpts{A11yLabel: "Small off", Small: true})
			Switch(c.Key("w5"), &f.on, SwitchOpts{A11yLabel: "Disabled", Disabled: true})
		})
		Slider(c.Key("sl1"), &f.vol, 0, 100, SliderOpts{A11yLabel: "Volume", ValueText: "40%"})
		Slider(c.Key("sl2"), &f.range0, 0, 100, SliderOpts{A11yLabel: "Seek", Range: theme.C("amber")})
		Slider(c.Key("sl3"), &f.vol, 0, 100, SliderOpts{A11yLabel: "Off", Disabled: true})
		ProgressBar(c.Key("p1"), 0.45, ProgressOpts{A11yLabel: "Download"})
		ProgressBar(c.Key("p2"), -1, ProgressOpts{A11yLabel: "Starting"})
		Segmented(c.Key("seg"), &f.seg, []SegmentOption[string]{{"list", "List"}, {"grid", "Grid"}, {"table", "Table"}}, "View")
		Tabs(c.Key("t1"), &f.tab, []string{"General", "Playback", "Advanced"}, TabsOpts{A11yLabel: "Settings"})
		Tabs(c.Key("t2"), &f.tabLine, []string{"Songs", "Albums", "Artists"}, TabsOpts{Line: true, A11yLabel: "Library"})
	})
}

func TestGoldenFields(t *testing.T) {
	f := newControlsFields()
	controlsSheet(t, "fields", 420, 780, f.view, nil)
}

func TestGoldenStates(t *testing.T) {
	notes := "Radiohead's third album, recorded in an Oxfordshire mansion and released in 1997, turned a band known for guitars towards drum machines, mellotrons and a colder, wider sound.\n\nIt was ranked among the greatest albums of its decade by critics and listeners alike, and has outlived most of the records it was compared with at the time.\nThird line of the second paragraph."
	artists := []services.Artist{{ID: "1", Name: "Daft Punk"}, {ID: "2", Name: "Pharrell Williams"}, {ID: "3", Name: "Nile Rodgers"}}
	controlsSheet(t, "states", 760, 1180, func(c *ui.Context) {
		PageHeader(c, "Albums", "128 albums, 42 hours", func() {
			Button(c.Key("sort"), "Sort", ButtonOpts{Variant: Outline, Size: SizeSM})
		})
		ui.Column(c.Key("empty")).Gap(16).Children(func() {
			EmptyState(c.Key("e1"), "No albums yet", "Add music from Apple Music and it appears here.", EmptyOpts{Action: &EmptyAction{Label: "Browse", Run: func() {}}})
			EmptyState(c.Key("e2"), "No albums yet", "Add music from Apple Music and it appears here.", EmptyOpts{Frame: FrameCovers})
			ErrorBlock(c.Key("err"), "Couldn't load this album", &net.DNSError{Err: "no such host"}, ErrorBlockOpts{OnRetry: func() {}})
		})
		ui.Row(c.Key("misc")).Gap(24).Children(func() {
			Notes(c.Key("notes"), notes, 3)
			ui.Text(c.Key("by"), "").Width(0)
			ui.Row(c.Key("al")).Children(func() { ArtistLinks(c, artists, "Daft Punk", ui.Color{}) })
		})
		DetailSkeleton(c, false)
	}, nil)
}

func TestGoldenOpen(t *testing.T) {
	f := newControlsFields()
	long := make([]SelectItem, 0, 40)
	for i := 1; i <= 40; i++ {
		v := fmt.Sprintf("Language %d", i)
		long = append(long, SelectOption(v, v))
	}
	lang := "Language 3"
	view := func(c *ui.Context) {
		ui.Row(c.Key("row")).Gap(16).Children(func() {
			Select(c.Key("plan"), &f.choice, []SelectItem{
				SelectLabel("Plans"), SelectOption("Free", "Free"), SelectOption("Pro", "Pro"), SelectSeparator(),
				SelectLabel("Teams"), SelectOption("Team", "Team"),
				{Value: "Ent", Label: "Enterprise", Disabled: true},
			}, SelectOpts{A11yLabel: "Plan"})
			Select(c.Key("lang"), &lang, long, SelectOpts{A11yLabel: "Language"})
		})
		Combobox(c.Key("combo"), &f.combo, []string{"France", "Frankfurt", "Freetown", "Germany"}, ComboboxOpts{A11yLabel: "Country", Width: 220})
		Button(c.Key("tip"), "", ButtonOpts{Icon: "ellipsis", Variant: Outline, A11yLabel: "More actions"})
	}
	controlsSheet(t, "open", 560, 420, view, func(tt *ui.Tester) {
		if err := tt.Click("Plan"); err != nil {
			t.Fatal(err)
		}
	})
	controlsSheet(t, "open-list", 560, 420, view, func(tt *ui.Tester) {
		if err := tt.Click("Language"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyDown)
	})
	controlsSheet(t, "open-combo", 560, 420, view, func(tt *ui.Tester) {
		if err := tt.Click("Country"); err != nil {
			t.Fatal(err)
		}
		tt.Key(ui.Cmd, ui.KeyA)
		tt.Type("fr")
	})
	controlsSheet(t, "tooltip", 560, 200, view, func(tt *ui.Tester) {
		r, ok := tt.Find("More actions")
		if !ok {
			t.Fatal("no More actions button")
		}
		tt.Move(r.X+r.W/2, r.Y+r.H/2)
		tt.Advance(time.Second)
	})
}

func TestGoldenCollectionHeader(t *testing.T) {
	l := newArtLoader(&fakeArt{}, nil, 1)
	t.Cleanup(l.Close)
	artists := []services.Artist{{ID: "1", Name: "Radiohead"}}
	header := func(c *ui.Context) {
		CollectionHeader(c, CollectionHeaderOpts{
			Title:  "OK Computer",
			Loader: l,
			Byline: func() { ArtistLinks(c, artists, "Radiohead", ui.Color{}) },
			Meta:   "Album · 1997 · 12 songs, 53 min",
			Badges: func() { Badges(c, BadgesOpts{Explicit: true, Traits: services.Traits{Lossless: true}}) },
			Description: "Radiohead's third album, recorded in an Oxfordshire mansion and released in 1997, turned a band known for guitars " +
				"towards drum machines, mellotrons and a colder, wider sound, and it was ranked among the greatest albums of its decade.",
			Actions: func() {
				PlayButtons(c, PlayButtonsOpts{Label: "OK Computer", Shuffle: true})
				LibraryButton(c, LibraryOpts{Name: "OK Computer", InLibrary: true})
				RatingButtons(c, RatingOpts{Name: "OK Computer", Rating: 1})
			},
		})
	}
	controlsSheet(t, "header", 900, 330, header, nil)
	controlsSheet(t, "header-wide", 1132, 330, header, nil)
}
