package cli

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"time"
)

// What the lyrics view of Sona does, the lines of `sona lyrics --follow`
// do: a line is shown a quarter of a second before it begins (so the eye
// arrives as the voice does), and it stays until the next begins, except
// when a pause of three seconds or more follows its end: then it goes half
// a second after the end and nothing is sung until the next line
// (internal/ui/lyricsync).
const (
	leadMs       = 250
	restGraceMs  = 500
	verseBreakMs = 3_000
)

// lineAt is which lyric line is sung at a playback position.
type lineAt struct {
	// Index is the line sung; -1 before the first line and while the voice
	// rests.
	Index int
	// Next is the line that begins next; -1 when none does.
	Next int
	// Until is the playback position, in seconds, at which Index or Next
	// next changes; +Inf when they never do.
	Until float64
}

// seconds is t in milliseconds, rounded: the unit of the lyrics' times.
func ms(t float64) int64 { return int64(math.Round(t * 1000)) }

// lineAtPosition finds the line of lines sung at pos seconds into the
// song: of the lines begun, the one begun last (the later of equal
// starts), until its end and a pause of verseBreakMs or more after it
// leaves nothing sung. It reads every line, so lines out of order give the
// same answer as sorted ones: lyrics come from outside and a misplaced line
// should not stop the display. Plain lyrics have no position: callers
// don't ask.
func lineAtPosition(lines []LyricLine, pos float64) lineAt {
	t := ms(max(pos, 0)) + leadMs
	cur, next := -1, -1
	var curBegin, nextBegin int64
	for i := range lines {
		b := ms(lines[i].Start)
		switch {
		case b <= t:
			if cur < 0 || b >= curBegin {
				cur, curBegin = i, b
			}
		case next < 0 || b <= nextBegin: // the later of equal starts, as cur
			next, nextBegin = i, b
		}
	}
	change := int64(math.MaxInt64) // when the answer changes
	if next >= 0 {
		change = nextBegin
	}
	if cur >= 0 {
		end := ms(lines[cur].End)
		if end > curBegin && (next < 0 || nextBegin-end >= verseBreakMs) {
			if rest := end + restGraceMs; t >= rest {
				cur = -1
			} else {
				change = min(change, rest)
			}
		}
	}
	until := math.Inf(1)
	if change != math.MaxInt64 {
		until = float64(change-leadMs) / 1000
	}
	return lineAt{Index: cur, Next: next, Until: until}
}

// verseStarts says which lines open a verse: after a pause (verseBreakMs),
// or where the song part changes. The first line opens none.
func verseStarts(lines []LyricLine) []bool {
	out := make([]bool, len(lines))
	for i := 1; i < len(lines); i++ {
		l, prev := &lines[i], &lines[i-1]
		if l.Part != "" && prev.Part != "" && l.Part != prev.Part {
			out[i] = true
			continue
		}
		end := ms(prev.End)
		out[i] = end > ms(prev.Start) && ms(l.Start)-end >= verseBreakMs
	}
	return out
}

// textMode is which text of a line is printed.
type textMode int

const (
	showOriginal textMode = iota
	showTranslation
	showTransliteration
)

func (in *invocation) textMode() textMode {
	switch {
	case in.translation:
		return showTranslation
	case in.transliteration:
		return showTransliteration
	}
	return showOriginal
}

var oneLiner = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ")

// shown is the line's text for printing, always on one line: its
// translation or transliteration when asked for and it has one, else the
// line itself (its background vocals when they are all it has).
func (l LyricLine) shown(m textMode) string {
	t := ""
	switch m {
	case showTranslation:
		t = l.Translation
	case showTransliteration:
		t = l.Transliteration
	}
	return oneLiner.Replace(cmp.Or(t, l.Text, l.Background))
}

// writeLyrics prints `sona lyrics`: plain text, or an LRC file with --lrc.
func writeLyrics(w io.Writer, in *invocation, l Lyrics) error {
	if in.lrc {
		return writeLRC(w, l, in.textMode())
	}
	var b strings.Builder
	starts := verseStarts(l.Lines)
	rest := false
	for i := range l.Lines {
		rest = rest || starts[i]
		text := l.Lines[i].shown(in.textMode())
		if text == "" {
			continue
		}
		if rest && b.Len() > 0 {
			b.WriteByte('\n')
		}
		rest = false
		b.WriteString(text + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeLRC writes l as an LRC file: the title, artist and album tags, then
// each line behind its start as [mm:ss.xx], in time order. Lyrics without
// timing have none to write: that is an error.
func writeLRC(w io.Writer, l Lyrics, m textMode) error {
	if !l.Synced {
		return &Error{Code: CodeNotSynced}
	}
	var b strings.Builder
	for _, tag := range []struct{ id, value string }{{"ti", l.Track.Title}, {"ar", l.Track.Artist}, {"al", l.Track.Album}} {
		if v := strings.TrimSpace(oneLiner.Replace(tag.value)); v != "" {
			fmt.Fprintf(&b, "[%s:%s]\n", tag.id, v)
		}
	}
	lines := slices.Clone(l.Lines)
	slices.SortStableFunc(lines, func(a, b LyricLine) int { return cmp.Compare(ms(a.Start), ms(b.Start)) })
	for i := range lines {
		fmt.Fprintf(&b, "%s%s\n", lrcStamp(lines[i].Start), lines[i].shown(m))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// lrcStamp is seconds as [mm:ss.xx], in hundredths cut off (the minutes
// run past 99 for a very long song).
func lrcStamp(seconds float64) string {
	t := max(ms(seconds), 0)
	return fmt.Sprintf("[%02d:%02d.%02d]", t/60_000, t/1000%60, t/10%100)
}

// Clock is the time `lyrics --follow` reads and the timer that wakes it at
// the next line. Env.Clock is nil in production, where it is the system's.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives once, d from now, and the
	// function that cancels it.
	After(d time.Duration) (<-chan time.Time, func())
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration) (<-chan time.Time, func()) {
	t := time.NewTimer(d)
	return t.C, func() { t.Stop() }
}

// minWake is the shortest wait before the next line: the position and the
// times of the lines are milliseconds, so a wake-up never needs to be
// sooner.
const minWake = time.Millisecond

// Lyrics states of a LyricsNow.
const (
	lyricsNone    = "none"
	lyricsLoading = "loading"
	lyricsReady   = "ready"
)

// lyricsFollower runs `lyrics --follow`. It never asks Sona for anything
// each line: the status stream says when the playback changes (the track, a
// pause, a seek), the lyrics are fetched once per song, and the line sung
// is worked out here from the position moved on by the clock, with one
// timer to the next boundary that runs only while the song plays.
type lyricsFollower struct {
	env Env
	in  *invocation
	t   Transport
	clk Clock

	status Status
	at     time.Time // when status was received: its position's moment

	lyrics   *Lyrics // of the song lyricsID; nil: none (or not fetched yet)
	lyricsID string  // the song lyrics belongs to ("" before the first)
	fetching string  // the song being fetched ("" when none)
	cancel   context.CancelFunc

	wake  <-chan time.Time
	unarm func()

	have    bool
	last    lyricsKey
	printed []byte
}

// lyricsKey is what a change of makes a new line: everything of an event
// but the position.
type lyricsKey struct {
	running  bool
	state    string
	track    string
	lyrics   string
	provider string
	index    int
	next     int
}

type fetched struct {
	id     string
	lyrics *Lyrics
	err    error
}

// followLyrics runs `lyrics --follow`: a line now and one each time the line
// sung changes, until Sona quits, then a last empty line.
func (env Env) followLyrics(ctx context.Context, in *invocation, t Transport) int {
	clk := env.Clock
	if clk == nil {
		clk = systemClock{}
	}
	f := &lyricsFollower{env: env, in: in, t: t, clk: clk, status: NotRunning()}
	ctx, stop := context.WithCancel(ctx)
	defer stop()

	statuses := make(chan Status)
	done := make(chan error, 1)
	go func() {
		done <- t.Follow(ctx, func(s Status) error {
			select {
			case statuses <- s:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	results := make(chan fetched)
	defer func() {
		if f.cancel != nil {
			f.cancel()
		}
		if f.unarm != nil {
			f.unarm()
		}
	}()

	for {
		select {
		case s := <-statuses:
			f.onStatus(ctx, s, results)
		case r := <-results:
			f.onFetched(r)
		case <-f.wake:
			f.wake, f.unarm = nil, nil
		case err := <-done:
			stop()
			if err == nil {
				if err := f.emitFinal(); err != nil {
					return env.fail(err)
				}
				return ExitOK
			}
			var cmdErr *Error
			if !errors.Is(err, ErrNotRunning) && !errors.As(err, &cmdErr) && f.have {
				err = errors.New(env.Printer.Sprintf(msgFollowEnded, err))
			}
			return env.unreachable(in, err)
		}
		if err := f.update(); err != nil {
			return env.fail(err)
		}
	}
}

// onStatus takes a status from Sona: the song changing starts its lyrics'
// fetch.
func (f *lyricsFollower) onStatus(ctx context.Context, s Status, results chan<- fetched) {
	f.status, f.at = s, f.clk.Now()
	id := ""
	if s.Track != nil {
		id = s.Track.ID
	}
	if id != "" && (id == f.lyricsID || id == f.fetching) {
		return
	}
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	f.lyrics, f.lyricsID, f.fetching = nil, "", ""
	if id == "" {
		return
	}
	f.fetching = id
	fctx, cancel := context.WithCancel(ctx)
	f.cancel = cancel
	go func() {
		r := fetched{id: id}
		cctx, done := context.WithTimeout(fctx, networkTimeout)
		defer done()
		raw, err := f.t.Call(cctx, Request{Command: "lyrics"})
		if err == nil {
			var l Lyrics
			if err = decode(raw, &l); err == nil {
				r.lyrics = &l
			}
		}
		r.err = err
		select {
		case results <- r:
		case <-fctx.Done():
		}
	}()
}

// onFetched takes the lyrics of the song being fetched. A song without
// lyrics, or whose lyrics couldn't be fetched, shows nothing; the second
// says why on standard error, once.
func (f *lyricsFollower) onFetched(r fetched) {
	if r.id != f.fetching {
		return
	}
	f.fetching = ""
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	if r.err == nil && r.lyrics.Track.ID != r.id {
		return // the song changed on its way: its own status follows
	}
	f.lyricsID, f.lyrics = r.id, r.lyrics
	var e *Error
	switch {
	case r.err == nil:
	case errors.As(r.err, &e) && (e.Code == CodeNoLyrics || e.Code == CodeNothingPlaying):
	case errors.As(r.err, &e):
		f.env.write(errorText(f.env.Printer, e))
	default:
		f.env.errorf(msgUnreachable, r.err)
	}
}

// position is how far into the song playback is now: the status's
// position moved on by the time since it, while playing.
func (f *lyricsFollower) position() float64 {
	pos := f.status.Position
	if f.status.State == StatePlaying {
		pos += f.clk.Now().Sub(f.at).Seconds()
		if t := f.status.Track; t != nil && t.Duration > 0 {
			pos = min(pos, t.Duration)
		}
	}
	return pos
}

// pastEnd reports that t seconds is at or past the playing song's end.
func (f *lyricsFollower) pastEnd(t float64) bool {
	tr := f.status.Track
	return tr != nil && tr.Duration > 0 && t >= tr.Duration
}

// event is what is sung now, and when that next changes (+Inf: not while
// it plays on its own).
func (f *lyricsFollower) event() (LyricsNow, lineAt) {
	pos := f.position()
	ev := LyricsNow{
		Running: f.status.Running, State: f.status.State, Track: f.status.Track,
		Position: math.Round(pos*1000) / 1000, Lyrics: lyricsNone, Index: -1,
	}
	at := lineAt{Index: -1, Next: -1, Until: math.Inf(1)}
	switch {
	case f.lyrics != nil:
		ev.Lyrics, ev.Provider, ev.Timing, ev.Synced = lyricsReady, f.lyrics.Provider, f.lyrics.Timing, f.lyrics.Synced
		if !f.lyrics.Synced {
			break
		}
		at = lineAtPosition(f.lyrics.Lines, pos)
		ev.Index = at.Index
		if at.Index >= 0 {
			ev.Line = &f.lyrics.Lines[at.Index]
			ev.Text = ev.Line.shown(f.in.textMode())
		}
		if at.Next >= 0 {
			ev.Next = &f.lyrics.Lines[at.Next]
		}
	case f.fetching != "":
		ev.Lyrics = lyricsLoading
	}
	return ev, at
}

// update prints the line when it changed and sets the timer for the next
// change: none while the song doesn't play, since then nothing moves.
func (f *lyricsFollower) update() error {
	ev, at := f.event()
	if f.unarm != nil {
		f.unarm()
		f.wake, f.unarm = nil, nil
	}
	// No wake for a change at or past the song's end: the next status (ended,
	// the next song) comes first, and position() stops there, so a timer
	// would only re-arm at the floor until it does.
	if f.status.State == StatePlaying && !math.IsInf(at.Until, 1) && !f.pastEnd(at.Until) {
		wait := max(time.Duration((at.Until-f.position())*float64(time.Second)), minWake)
		f.wake, f.unarm = f.clk.After(wait)
	}
	key := lyricsKey{
		running: ev.Running, state: ev.State, lyrics: ev.Lyrics, provider: ev.Provider,
		index: at.Index, next: at.Next,
	}
	if ev.Track != nil {
		key.track = ev.Track.ID
	}
	if f.have && key == f.last {
		return nil
	}
	f.have, f.last = true, key
	return f.write(ev, false)
}

// emitFinal prints the last line, once Sona is gone: always, even when it
// repeats the one before, so that whatever shows the lines clears.
func (f *lyricsFollower) emitFinal() error {
	if f.unarm != nil {
		f.unarm()
		f.wake, f.unarm = nil, nil
	}
	return f.write(lyricsGone(), true)
}

// lyricsGone is the last line of `lyrics --follow`: Sona isn't running.
func lyricsGone() LyricsNow { return LyricsNow{State: StateIdle, Lyrics: lyricsNone, Index: -1} }

// write prints ev the way the invocation asks. A line identical to the one
// before (a change it doesn't show, such as a pause in the text) is left out
// unless forced.
func (f *lyricsFollower) write(ev LyricsNow, force bool) error {
	var b bytes.Buffer
	if err := writeLyricsNow(&b, f.in, ev); err != nil {
		return err
	}
	if !force && bytes.Equal(b.Bytes(), f.printed) {
		return nil
	}
	f.printed = bytes.Clone(b.Bytes())
	_, err := f.env.Stdout.Write(b.Bytes())
	return err
}

// writeLyricsNow prints one line of `lyrics --follow`: its JSON, its
// template, or the text of the line sung.
func writeLyricsNow(w io.Writer, in *invocation, ev LyricsNow) error {
	switch {
	case in.json:
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		return writeLine(w, string(b))
	case in.format != "":
		t, err := statusTemplate(in.format)
		if err != nil {
			return err
		}
		return executeTemplate(w, t, ev)
	}
	return writeLine(w, ev.Text)
}
