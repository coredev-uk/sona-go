// Package measure is what Sona's UI probes share: MyGo's frame statistics
// (MYGO_FRAME_STATS=all) parsed into frames, the summary of a scroll, the
// process tree's CPU and memory, a client of MyGo's automation socket, and
// the Albums scroll scenario that `sona -probe` runs in-process (package
// probe, build tag probe) and cmd/uiprobe runs from outside, so both print
// the same table.
//
// Frame times come from MyGo's own log: every presented frame writes one
// "mygo: frame N took … ms: build …, layout …, paint …, present …" line.
// A frame exists only when something changed, so the gaps between frames
// during a scroll are the dropped frames.
package measure

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Frame is one presented frame.
type Frame struct {
	// At is when the frame was logged, which is when it finished.
	At time.Time
	// N is MyGo's frame counter.
	N int
	// Took is the whole frame, and Build, Layout, Paint and Present its
	// parts, in milliseconds. Present is the hand-over to the display
	// (with the CPU renderer, the copy into the compositor's buffer).
	Took, Build, Layout, Paint, Present float64
	// Path says how the frame was drawn ("drawn on the GPU", "drawn in
	// memory").
	Path string
}

// Work is the CPU time the frame took before presenting.
func (f Frame) Work() float64 { return f.Build + f.Layout + f.Paint }

var frameLine = regexp.MustCompile(`mygo: frame (\d+) took ([\d.]+) ms: build ([\d.]+)(?: \(\d+ passes\)| \(painted again\))?, layout ([\d.]+), paint ([\d.]+), present ([\d.]+) \(([^)]*)\)`)

// ParseFrame reads a MyGo frame statistics line. at is when it was logged.
func ParseFrame(line string, at time.Time) (Frame, bool) {
	m := frameLine.FindStringSubmatch(line)
	if m == nil {
		return Frame{}, false
	}
	f := Frame{At: at, Path: m[7]}
	f.N, _ = strconv.Atoi(m[1])
	for i, p := range []*float64{&f.Took, &f.Build, &f.Layout, &f.Paint, &f.Present} {
		*p, _ = strconv.ParseFloat(m[2+i], 64)
	}
	return f, true
}

// Frames is where a scenario reads the frames drawn so far.
type Frames interface {
	// Count is how many frames there are; a Mark.
	Count() int
	// Since is the frames from mark on, oldest first.
	Since(mark int) []Frame
}

// Recorder collects frames as they are logged. It is a Frames, and an
// io.Writer for the standard logger: each line that is a frame statistics
// line is recorded, any other goes to Other.
type Recorder struct {
	// Other receives the log lines that are not frames; nil drops them.
	Other io.Writer

	mu     sync.Mutex
	frames []Frame
}

// Write takes the standard logger's output, one line per call.
func (r *Recorder) Write(b []byte) (int, error) {
	if f, ok := ParseFrame(string(b), time.Now()); ok {
		r.mu.Lock()
		r.frames = append(r.frames, f)
		r.mu.Unlock()
		return len(b), nil
	}
	if r.Other != nil {
		return r.Other.Write(b)
	}
	return len(b), nil
}

// Count implements Frames.
func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.frames)
}

// Since implements Frames.
func (r *Recorder) Since(mark int) []Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Frame(nil), r.frames[min(mark, len(r.frames)):]...)
}

// LogFrames follows the frame statistics lines an app appends to its JSON
// log (sona.log, with MYGO_FRAME_STATS=all in its environment): the frames
// written after Follow was called.
type LogFrames struct {
	path string

	mu      sync.Mutex
	off     int64
	partial []byte
	rec     Recorder
}

// Follow reads the log at path from byte offset off.
func Follow(path string, off int64) *LogFrames {
	return &LogFrames{path: path, off: off}
}

// LastStart finds the last start of the app in its JSON log: when the
// process began (the "startup: process start" line), its pid and the byte
// offset of that line, from where Follow reads the frames of that run.
func LastStart(path string) (at time.Time, pid int, off int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, 0, 0, err
	}
	defer func() { _ = f.Close() }()
	r := bufio.NewReaderSize(f, 1<<20)
	var pos int64
	for {
		line, rerr := r.ReadBytes('\n')
		var rec struct {
			Time time.Time `json:"time"`
			Msg  string    `json:"msg"`
			PID  int       `json:"pid"`
		}
		if json.Unmarshal(line, &rec) == nil && rec.Msg == "startup: process start" {
			at, pid, off = rec.Time, rec.PID, pos
		}
		pos += int64(len(line))
		if rerr != nil {
			break
		}
	}
	if at.IsZero() {
		return at, 0, 0, fmt.Errorf("%s has no `startup: process start` line: not a Sona log", path)
	}
	return at, pid, off, nil
}

// refresh reads what the app appended since the last call.
func (l *LogFrames) refresh() {
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.Open(l.path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return
	}
	if fi.Size() < l.off { // rotated: a new file
		l.off, l.partial = 0, nil
	}
	if _, err := f.Seek(l.off, io.SeekStart); err != nil {
		return
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	l.off += int64(len(data))
	data = append(l.partial, data...)
	l.partial = nil
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		var rec struct {
			Time time.Time `json:"time"`
			Msg  string    `json:"msg"`
		}
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if f, ok := ParseFrame(rec.Msg, rec.Time); ok {
			l.rec.mu.Lock()
			l.rec.frames = append(l.rec.frames, f)
			l.rec.mu.Unlock()
		}
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		// An unfinished last line: keep it for the next read.
		if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
			l.partial = append([]byte(nil), data[i+1:]...)
		} else {
			l.partial = append([]byte(nil), data...)
		}
	}
}

// Count implements Frames.
func (l *LogFrames) Count() int {
	l.refresh()
	return l.rec.Count()
}

// Since implements Frames.
func (l *LogFrames) Since(mark int) []Frame {
	l.refresh()
	return l.rec.Since(mark)
}
