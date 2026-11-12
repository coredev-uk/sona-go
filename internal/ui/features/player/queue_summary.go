package player

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// summaryOf is the line on what follows the current song ("9 songs after
// this one"), "" for an empty queue.
func summaryOf(s services.PlaybackState, q services.QueueState) string {
	parts, start := partsOf(s, q)
	return summaryFor(parts, start, q)
}

// summaryFor is summaryOf over parts already worked out.
func summaryFor(parts queueParts, start int, q services.QueueState) string {
	autoRows := parts.auto > 0 && q.Station != nil && q.Station.AutoPlay
	switch {
	case len(q.Items) == 0:
		return ""
	case start < len(q.Items) && q.Items[start].Live && parts.mine == 0:
		return i18n.T("Live radio")
	case parts.mine > 0 && autoRows:
		return i18n.T("{mine, plural, one {# song after this one, then AutoPlay} other {# songs after this one, then AutoPlay}}", "mine", parts.mine)
	case parts.mine > 0:
		return i18n.T("{mine, plural, one {# song after this one} other {# songs after this one}}", "mine", parts.mine)
	case autoRows:
		// translators: Queue summary when only AutoPlay's songs follow. Apple's ASE.Web.Music.UpNext.Heading.AutoplayActive ('autoplay is on') is lower case, for a subline, so it is not mapped.
		return i18n.T("AutoPlay is on")
	case parts.auto > 0:
		return i18n.T("Nothing queued after this song")
	}
	return i18n.T("Nothing after this song")
}

// queueSummary is one line on what follows the current song, or the station
// it plays from, as a link (leave, when given, runs as it is followed: Now
// Playing closes).
func (p *player) queueSummary(c *ui.Context, leave func()) {
	q := p.env.Play.Queue()
	tk := theme.Current()
	muted := theme.C("ink-muted")
	if st := q.Station; st != nil && !st.AutoPlay {
		p.playingFrom(c, st, leave)
		return
	}
	parts, start := p.queueParts(p.env.Play.State(), q)
	text := summaryFor(parts, start, q)
	if text == "" {
		return
	}
	t := tk.Type["meta"]
	// Proportional figures: tabular ones widen the comma in this sentence.
	ui.Text(c.Key("summary"), text).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(muted).SingleLine().MinWidth(0).Shrink(1)
}

// playingFrom is "Playing from <station>": the station's name links to its
// page when that page is ported.
func (p *player) playingFrom(c *ui.Context, st *services.QueueStation, leave func()) {
	const open, closeTag = "<0>", "</0>"
	whole := i18n.T("Playing from <0>{playingFrom}</0>", "playingFrom", st.Name)
	i, j := strings.Index(whole, open), strings.Index(whole, closeTag)
	t := theme.Current().Type["meta"]
	muted := theme.C("ink-muted")
	text := func(key, s string, col ui.Color) ui.Element {
		return ui.Text(c.Key(key), s).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(col).SingleLine().Shrink(0)
	}
	if i < 0 || j < i {
		ui.Text(c.Key("summary"), whole).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(muted).SingleLine().MinWidth(0).Shrink(1)
		return
	}
	path := "/station/" + st.ID
	ui.Row(c.Key("summary")).MinWidth(0).Shrink(1).Children(func() {
		if i > 0 {
			text("before", whole[:i], muted)
		}
		name := text("station", whole[i+len(open):j], theme.C("ink")).Shrink(1).MinWidth(0)
		if p.env.Routed(path) {
			name.Role(ui.RoleLink).Cursor(ui.CursorPointer).Underline().DecorationColor(theme.C("ink").Alpha(0.4))
			if name.Hovered() {
				name.DecorationColor(theme.C("ink"))
			}
			if name.Clicked() {
				if leave != nil {
					leave()
				}
				p.env.Nav.Push(path)
			}
		}
		if rest := whole[j+len(closeTag):]; rest != "" {
			text("after", rest, muted)
		}
	})
}
