package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"text/template"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// stateText is a status state as printed.
func stateText(p *l10n.Printer, state string) string {
	switch state {
	case StateLoading:
		return p.T(msgStateLoading)
	case StatePlaying:
		return p.T(msgStatePlaying)
	case StatePaused:
		return p.T(msgStatePaused)
	case StateEnded:
		return p.T(msgStateEnded)
	case StateError:
		return p.T(msgStateError)
	}
	return p.T(msgStateIdle)
}

func onOff(p *l10n.Printer, on bool) string {
	if on {
		return p.T(msgOn)
	}
	return p.T(msgOff)
}

// clock writes seconds as m:ss, or h:mm:ss from an hour.
func clock(seconds float64) string {
	s := int(math.Max(seconds, 0))
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func byArtist(p *l10n.Printer, title, artist string) string {
	if artist == "" {
		return title
	}
	return p.Sprintf(msgTrackByArtist, title, artist)
}

// summary is the one-line status: what play, pause, toggle, next and prev
// print, and each line of `status --follow`.
func summary(p *l10n.Printer, s Status) string {
	if !s.Running {
		return p.T(msgNotRunningState)
	}
	line := stateText(p, s.State)
	if s.Track != nil {
		line = p.Sprintf(msgFieldLine, line, byArtist(p, s.Track.Title, s.Track.Artist))
	}
	if s.Preview {
		line += p.T(msgPreviewSuffix)
	}
	return line
}

// writeStatus prints `sona status`: one field per line.
func writeStatus(w io.Writer, p *l10n.Printer, s Status) error {
	if !s.Running {
		_, err := fmt.Fprintln(w, p.T(msgStatusNotRunning))
		return err
	}
	var b strings.Builder
	field := func(k l10n.Message, v string) { b.WriteString(p.Sprintf(msgFieldLine, p.T(k), v) + "\n") }
	field(msgFieldState, stateText(p, s.State))
	if t := s.Track; t != nil {
		field(msgFieldTitle, t.Title)
		if t.Artist != "" {
			field(msgFieldArtist, t.Artist)
		}
		if t.Album != "" {
			field(msgFieldAlbum, t.Album)
		}
		pos := clock(s.Position)
		if t.Duration > 0 {
			pos = p.Sprintf(msgPositionOf, pos, clock(t.Duration))
		}
		field(msgFieldPosition, pos)
	}
	field(msgFieldVolume, p.Sprintf(msgPercent, s.Volume))
	field(msgFieldShuffle, onOff(p, s.Shuffle))
	field(msgFieldRepeat, repeatText(p, s.Repeat))
	field(msgFieldPrivate, onOff(p, s.Private))
	if s.Preview {
		field(msgFieldPreview, p.T(msgYes))
	}
	if s.Error != "" {
		field(msgFieldError, s.Error)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func repeatText(p *l10n.Printer, r string) string {
	switch r {
	case "all":
		return p.T(msgRepeatAll)
	case "one":
		return p.T(msgRepeatOne)
	}
	return p.T(msgOff)
}

func writeQueue(w io.Writer, p *l10n.Printer, q Queue) error {
	if len(q.Tracks) == 0 {
		_, err := fmt.Fprintln(w, p.T(msgQueueEmpty))
		return err
	}
	width := len(fmt.Sprint(len(q.Tracks)))
	var b strings.Builder
	for i, t := range q.Tracks {
		mark := " "
		if i == q.Index {
			mark = queueMark
		}
		line := byArtist(p, t.Title, t.Artist)
		if t.AutoPlay {
			line += " " + p.T(msgQueueAutoPlay)
		}
		fmt.Fprintf(&b, "%s %*d  %s\n", mark, width, i+1, line)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeSearch(w io.Writer, p *l10n.Printer, r SearchResults) error {
	if len(r.Results) == 0 {
		_, err := fmt.Fprintln(w, p.Sprintf(msgSearchNone, r.Term))
		return err
	}
	var b strings.Builder
	kind := ""
	for _, it := range r.Results {
		if it.Kind != kind {
			kind = it.Kind
			heading := kind
			if m, ok := msgKinds[kind]; ok {
				heading = p.T(m)
			}
			b.WriteString(heading + "\n")
		}
		line := byArtist(p, it.Name, it.Subtitle)
		if it.URL != "" {
			line += "  " + it.URL
		}
		b.WriteString("  " + line + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// privateLine is `sona private`'s line: on or off, then what decides it
// when that isn't the saved setting.
func privateLine(p *l10n.Printer, pv Private) string {
	line := p.Sprintf(msgPrivateLine, onOff(p, pv.On))
	switch pv.Source {
	case SourceDevelopment:
		line += p.Sprintf(msgPrivateDev, onOff(p, pv.Saved))
	case SourceEnvironment:
		line += p.T(msgPrivateForcedEnv)
	}
	return line
}

func likeLine(p *l10n.Printer, l Like) string {
	song := byArtist(p, l.Track.Title, l.Track.Artist)
	if l.Liked {
		return p.Sprintf(msgLiked, song)
	}
	return p.Sprintf(msgNotLiked, song)
}

// statusTemplate parses a --format template. Its data is the status's
// JSON object, so fields are the JSON names (.state, .track.title).
func statusTemplate(text string) (*template.Template, error) {
	return template.New("status").Option("missingkey=zero").Funcs(template.FuncMap{
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"clock": func(v any) string {
			f, _ := v.(float64)
			return clock(f)
		},
	}).Parse(text)
}

// executeTemplate writes s through t, with a newline after it.
func executeTemplate(w io.Writer, t *template.Template, s Status) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return err
	}
	b.WriteByte('\n')
	_, err = io.WriteString(w, b.String())
	return err
}
