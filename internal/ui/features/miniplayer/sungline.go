package miniplayer

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// sungState is what the sung line derives from a song's lyrics, once per
// lyrics (the query hands out a copy of the DTO each frame, so the lines'
// backing array says whether they are the same lyrics).
type sungState struct {
	first *services.LyricLine
	n     int
	spans []lyricsync.Span
}

// spansOf are the lines' spans, worked out again only for other lyrics.
func (s *sungState) spansOf(l *services.Lyrics) []lyricsync.Span {
	var first *services.LyricLine
	if len(l.Lines) > 0 {
		first = &l.Lines[0]
	}
	if s.first != first || s.n != len(l.Lines) || s.spans == nil {
		s.first, s.n, s.spans = first, len(l.Lines), lyricsync.LineSpans(l.Lines)
	}
	return s.spans
}

func (m *mini) lyricsSvc() lyricsSource {
	if m.lyr != nil {
		return m.lyr
	}
	return services.Service[services.LyricsService](m.env.Core)
}

// sungLine is the line being sung for the third line (v0.2.0's SungLine): up
// to two lines of text marked with the lyrics glyph, its words lit whole in
// amber as they are sung (no fill: a glance needs no frames). Between lines,
// in a break and for songs without synced lyrics it shows fallback (the
// album) instead. A timer wakes the view at the next line or word; nothing
// polls.
func (m *mini) sungLine(c *ui.Context, cur *services.QueueItem, playing bool, fallback string) {
	line, lang, lit := m.lineSung(c, cur, playing)
	if line == nil {
		quiet(c.Key("fallback"), fallback).TextColor(theme.C("ink-faint"))
		return
	}
	t := theme.Current().Type["sub"]
	ui.Row(c.Key("sung")).FillWidth().Gap(4).AlignItems(ui.Start).MinWidth(0).Children(func() {
		ui.Icon(c.Key("mic"), theme.Icon("mic-vocal")).FontSize(12).TextColor(theme.C("ink-faint")).Margin(3, 0, 0, 0).Shrink(0)
		ink, amber := theme.C("ink"), theme.C("amber-ink")
		spans := make([]ui.Span, 0, len(line.Words)+1)
		if len(line.Words) == 0 {
			spans = append(spans, ui.Span{Text: line.Text})
		}
		for i, w := range line.Words {
			col := ink
			if i < lit {
				col = amber
			}
			spans = append(spans, ui.Span{Text: w.Text, Color: col})
		}
		ui.RichText(c.Key("line"), spans...).Lang(lang).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
			TextColor(ink).MaxLines(2).MinWidth(0).Shrink(1).Grow(1)
	})
}

// lineSung is the lead of the line sung now, in the lyrics' language, with
// how many of its words have begun (all of them for a line without word
// timing), nil when there is none to show: no lyrics, unsynced ones, a rest
// between lines. It asks for the frame at which that next changes.
func (m *mini) lineSung(c *ui.Context, cur *services.QueueItem, playing bool) (*lyricsync.Text, string, int) {
	songID := lyricsync.SongID(cur)
	if songID == "" {
		return nil, "", 0
	}
	svc := m.lyricsSvc()
	r := store.UseLyricsFrom(m.env.Queries, songID, func(ctx context.Context) (services.Lyrics, error) {
		return svc.Lyrics(ctx, songID)
	})
	if !r.HasData || !lyricsync.IsSynced(&r.Data) || len(r.Data.Lines) == 0 {
		return nil, "", 0
	}
	l := &r.Data
	spans := m.sung.spansOf(l)
	posMs := m.env.Play.Position().Milliseconds()
	off := lyricsync.Session().Get(songID)
	clock := lyricsync.LyricsClock(posMs, off)
	sung := lyricsync.SungLine(spans, clock)
	next := lyricsync.Never // until the line, a rest or a word changes
	if sung.Next != lyricsync.Never {
		next = sung.Next - clock
	}

	var out *lyricsync.Text
	lit := 0
	if sung.Active >= 0 {
		shown := lyricsync.Shown(&l.Lines[sung.Active], lyricsync.Prefs{})
		if shown.Lead.Text != "" || len(shown.Lead.Words) > 0 {
			out = &shown.Lead
		}
		// Words light whole at their begin, on the word clock.
		var wordNext int64
		lit, wordNext = litWords(shown.Lead.Words, lyricsync.WordClock(posMs, off))
		next = min(next, wordNext)
	}
	if playing && next != lyricsync.Never {
		c.After(msDur(max(next, 0) + 1))
	}
	return out, l.Language, lit
}

// litWords is how many of a line's words have begun at word-clock time t (the
// mini player lights each whole at its begin), and how long until the next
// one does (lyricsync.Never when all have).
func litWords(words []services.LyricWord, t int64) (lit int, next int64) {
	next = lyricsync.Never
	for _, w := range words {
		if w.BeginMs > t {
			return lit, w.BeginMs - t
		}
		lit++
	}
	return lit, next
}
