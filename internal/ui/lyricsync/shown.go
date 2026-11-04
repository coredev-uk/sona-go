package lyricsync

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Prefs is what the lyrics view adds under the sung words.
type Prefs struct {
	// Translation: the line translated into the chosen language
	// (Settings → Playback → Lyrics).
	Translation bool
	// Pronunciation: the words in Latin script (romanisation).
	Pronunciation bool
}

// Text is words, or plain text where the lyrics have no word timing.
type Text struct {
	Text string
	// Words are the timed words; empty for untimed text.
	Words []services.LyricWord
}

// ShownLine is one line as the view shows it under the current choices.
type ShownLine struct {
	// Lead is the main vocal.
	Lead Text
	// Background is the background vocals (empty text when there are none).
	Background Text
	// Pronunciation is the line in Latin script, under it; nil when not
	// shown or missing.
	Pronunciation *Text
	// Translation is the translated line, "" when not shown or missing.
	Translation string
}

func joined(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// Shown is a line under prefs: its main vocal and background vocals (each
// with its own words), then its romanisation and translation beneath when
// they are on and the line has them.
func Shown(line *services.LyricLine, prefs Prefs) ShownLine {
	out := ShownLine{
		Lead:       Text{Text: line.Text},
		Background: Text{Text: line.Background},
	}
	for _, w := range line.Words {
		if w.Background {
			out.Background.Words = append(out.Background.Words, w)
		} else {
			out.Lead.Words = append(out.Lead.Words, w)
		}
	}
	if tl := line.Transliteration; prefs.Pronunciation && tl != nil && (tl.Text != "" || tl.Background != "") {
		out.Pronunciation = &Text{Text: joined(tl.Text, tl.Background), Words: tl.Words}
	}
	if tr := line.Translation; prefs.Translation && tr != nil {
		out.Translation = joined(tr.Text, tr.Background)
	}
	return out
}
