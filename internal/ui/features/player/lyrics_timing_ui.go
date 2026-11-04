package player

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
)

// describeOffset is the offset in words: "0.5 s earlier", "1 s later", ""
// when there is none (lyrics.ts describeOffset, one decimal at most).
func describeOffset(offsetMs int64) string {
	if offsetMs == 0 {
		return ""
	}
	s := i18n.FormatNumber(math.Round(math.Abs(float64(offsetMs))/100) / 10)
	if offsetMs < 0 {
		return i18n.T("{s} s earlier", "s", s)
	}
	return i18n.T("{s} s later", "s", s)
}

// lyricsTiming is the timing nudge of one song (LyricsTiming.tsx): earlier
// and later by half a second, and the offset in words between them (pressing
// it puts the lyrics back on Apple's timing). Kept per song for the session.
func (p *player) lyricsTiming(c *ui.Context, songID string) {
	offset := lyricsync.Session().Get(songID)
	words := describeOffset(offset)
	step := i18n.FormatNumber(float64(lyricsync.OffsetStepMs) / 1000)
	ui.Row(c.Key("lyrics-timing")).AlignItems(ui.Center).Role(ui.RoleGroup).Label(i18n.T("Lyrics timing")).Children(func() {
		earlier := i18n.T("Show lyrics {step} s earlier", "step", step)
		if kit.Button(c.Key("earlier"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Icon: "minus", Muted: true, A11yLabel: earlier}).Clicked() {
			lyricsync.Session().Nudge(songID, -1)
		}
		label, tip := i18n.T("Lyrics on time"), ""
		if words != "" {
			label, tip = i18n.T("Lyrics {words}", "words", words), i18n.T("Back to the original timing")
		}
		// Announced as it changes, so a nudge from the keyboard is heard.
		if kit.Button(c.Key("offset"), label, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Muted: true, Disabled: words == "", Tip: tip}).
			Role(ui.RoleStatus).MinWidth(120).Clicked() {
			lyricsync.Session().Reset(songID)
		}
		later := i18n.T("Show lyrics {step} s later", "step", step)
		if kit.Button(c.Key("later"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Icon: "plus", Muted: true, A11yLabel: later}).Clicked() {
			lyricsync.Session().Nudge(songID, 1)
		}
	})
}
