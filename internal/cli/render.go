package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"text/template"
)

// stateText is a status state as printed.
func stateText(state string) string {
	switch state {
	case StateLoading:
		return msgStateLoading
	case StatePlaying:
		return msgStatePlaying
	case StatePaused:
		return msgStatePaused
	case StateEnded:
		return msgStateEnded
	case StateError:
		return msgStateError
	}
	return msgStateIdle
}

func onOff(on bool) string {
	if on {
		return msgOn
	}
	return msgOff
}

// clock writes seconds as m:ss, or h:mm:ss from an hour.
func clock(seconds float64) string {
	s := int(math.Max(seconds, 0))
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func byArtist(title, artist string) string {
	if artist == "" {
		return title
	}
	return fmt.Sprintf(msgTrackByArtist, title, artist)
}

// summary is the one-line status: what play, pause, toggle, next and prev
// print, and each line of `status --follow`.
func summary(s Status) string {
	if !s.Running {
		return msgNotRunningState
	}
	line := stateText(s.State)
	if s.Track != nil {
		line = fmt.Sprintf(msgTrackLine, line, byArtist(s.Track.Title, s.Track.Artist))
	}
	if s.Preview {
		line += msgPreviewSuffix
	}
	return line
}

// writeStatus prints `sona status`: one field per line.
func writeStatus(w io.Writer, s Status) error {
	if !s.Running {
		_, err := fmt.Fprintln(w, msgStatusNotRunning)
		return err
	}
	var b strings.Builder
	field := func(k, v string) { fmt.Fprintf(&b, "%s: %s\n", k, v) }
	field(msgFieldState, stateText(s.State))
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
			pos = fmt.Sprintf(msgPositionOf, pos, clock(t.Duration))
		}
		field(msgFieldPosition, pos)
	}
	field(msgFieldVolume, fmt.Sprintf(msgPercent, s.Volume))
	field(msgFieldShuffle, onOff(s.Shuffle))
	field(msgFieldRepeat, repeatText(s.Repeat))
	field(msgFieldPrivate, onOff(s.Private))
	if s.Preview {
		field(msgFieldPreview, msgYes)
	}
	if s.Error != "" {
		field(msgFieldError, s.Error)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func repeatText(r string) string {
	switch r {
	case "all":
		return msgRepeatAll
	case "one":
		return msgRepeatOne
	}
	return msgOff
}

func writeQueue(w io.Writer, q Queue) error {
	if len(q.Tracks) == 0 {
		_, err := fmt.Fprintln(w, msgQueueEmpty)
		return err
	}
	width := len(fmt.Sprint(len(q.Tracks)))
	var b strings.Builder
	for i, t := range q.Tracks {
		mark := " "
		if i == q.Index {
			mark = msgQueueCurrent
		}
		line := byArtist(t.Title, t.Artist)
		if t.AutoPlay {
			line += " " + msgQueueAutoPlay
		}
		fmt.Fprintf(&b, "%s %*d  %s\n", mark, width, i+1, line)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func writeSearch(w io.Writer, r SearchResults) error {
	if len(r.Results) == 0 {
		_, err := fmt.Fprintf(w, msgSearchNone+"\n", r.Term)
		return err
	}
	var b strings.Builder
	kind := ""
	for _, it := range r.Results {
		if it.Kind != kind {
			kind = it.Kind
			heading, ok := msgKinds[kind]
			if !ok {
				heading = kind
			}
			b.WriteString(heading + "\n")
		}
		line := byArtist(it.Name, it.Subtitle)
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
func privateLine(p Private) string {
	line := fmt.Sprintf(msgPrivateLine, onOff(p.On))
	switch p.Source {
	case SourceDevelopment:
		line += fmt.Sprintf(msgPrivateDev, onOff(p.Saved))
	case SourceEnvironment:
		line += msgPrivateForcedEnv
	}
	return line
}

func likeLine(l Like) string {
	song := byArtist(l.Track.Title, l.Track.Artist)
	if l.Liked {
		return fmt.Sprintf(msgLiked, song)
	}
	return fmt.Sprintf(msgNotLiked, song)
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
