package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// releaseNotesWidth caps the dialog's panel: its 24 DIP padding leaves the
// text about 70 characters of body type across, the measure to read a list of
// changes in without losing the line.
const releaseNotesWidth = 600

// releaseNotesChrome is what the panel holds besides the notes (its padding,
// header, footer and the gaps between): the notes scroll in what the window
// leaves of it.
const releaseNotesChrome = 240

// ReleaseNotesAction is a button of the dialog's footer.
type ReleaseNotesAction struct {
	Label string
	// Icon is a Lucide name, before the label (after it with IconAfter: the
	// external-link mark of an action that leaves Sona).
	Icon      string
	IconAfter bool
	// Run is what the button does, on the main thread; the dialog stays open
	// unless Run closes it.
	Run func()
}

// ReleaseNotesSpec is what ReleaseNotes.Show shows.
type ReleaseNotesSpec struct {
	// Version is the release's number ("0.8.0"); Published its day (zero
	// when unknown, and then the dialog says no date).
	Version   string
	Published time.Time
	// Notes is the release's notes (Markdown); empty says there are none.
	Notes string
	// Primary and Secondary are the footer's actions beside Close, Primary
	// last and in the ink fill ("Install and restart"), Secondary an outline
	// before it ("View release"); one with no Run is left out.
	Primary, Secondary ReleaseNotesAction
}

// ReleaseNotes is the dialog that shows a release's notes: "What's new in
// Sona {version}" over the date, the notes as Markdown in a scroll area, and
// the footer's Close with the actions the caller gives. The update notice
// opens it for the update on offer; the What's New window after an update
// (#675) shows the version just installed in it.
//
// Keep one in the state of the module that hosts the dialog, and call Show on
// every build, whether it is open or not: it builds nothing while it is closed.
// It must not be copied.
type ReleaseNotes struct {
	md *Markdown
}

// NewReleaseNotes is a ReleaseNotes whose links open through open (see
// NewMarkdown): the app's one way to open a web page.
func NewReleaseNotes(open func(url string)) *ReleaseNotes {
	return &ReleaseNotes{md: NewMarkdown(open)}
}

// Show builds the dialog while *open is true and returns its panel (a zero
// Element while closed). The dialog is a modal "dialog" named by its title,
// at most releaseNotesWidth wide, whose notes scroll within the window; Escape,
// the X and Close set *open to false and give the focus back to what opened it.
// Links in the notes open in the browser and take the keyboard focus, so Tab
// reaches them and the arrow keys, Page Up and Page Down scroll the notes
// around the focus.
func (r *ReleaseNotes) Show(c *ui.Context, open *bool, s ReleaseNotesSpec) ui.Element {
	o := DialogOpts{
		Title:        i18n.T("What's new in Sona {version}", "version", s.Version),
		MaxWidth:     releaseNotesWidth,
		HeadingTitle: true,
	}
	if !s.Published.IsZero() {
		o.Description = i18n.T("Released {date}", "date", i18n.FormatDate(s.Published, i18n.DateLong))
	}
	return Dialog(c, open, o, func() {
		_, h := c.Size()
		if s.Notes == "" {
			theme.Text(c.Key("notes:none"), i18n.T("This release has no notes. The release page may say more."), "body").TextColor(theme.C("ink-muted"))
		} else {
			// Padding keeps the text off the scroll bar, which overlays it.
			ui.Scroll(c.Key("notes:body")).FillWidth().MaxHeight(max(h-releaseNotesChrome, 160)).Padding(0, 12, 0, 0).
				Focusable().Role(ui.RoleGroup).Label(i18n.T("Release notes")).Children(func() {
				r.md.View(c.Key("notes:text"), s.Notes)
			})
		}
		ui.Row(c.Key("notes:footer")).Gap(8).Justify(ui.End).AlignItems(ui.Center).FillWidth().Wrap().Children(func() {
			if Button(c.Key("notes:close"), i18n.T("Close"), ButtonOpts{Variant: Outline}).Clicked() {
				*open = false
			}
			for i, a := range []ReleaseNotesAction{s.Secondary, s.Primary} {
				if a.Run == nil {
					continue
				}
				variant := Outline
				if i == 1 {
					variant = Default
				}
				if Button(c.Key("notes:action:"+a.Label), a.Label, ButtonOpts{Variant: variant, Icon: a.Icon, IconAfter: a.IconAfter}).Clicked() {
					a.Run()
				}
			}
		})
	})
}
