package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"time"
)

// logLine is a line of Sona's JSON log.
type logLine struct {
	Time  time.Time `json:"time"`
	Msg   string    `json:"msg"`
	Since int64     `json:"since_start_ms"`
}

// readLog calls fn for each line of Sona's JSON log that parses.
func readLog(name string, fn func(logLine)) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var l logLine
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			fn(l)
		}
	}
	return sc.Err()
}

// firstPaints are the "startup: first paint" times (since process start) of
// Sona's JSON logs.
func firstPaints(files ...string) ([]time.Duration, error) {
	var out []time.Duration
	for _, name := range files {
		if err := readLog(name, func(l logLine) {
			if l.Msg == "startup: first paint" {
				out = append(out, time.Duration(l.Since)*time.Millisecond)
			}
		}); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no `startup: first paint` lines")
	}
	return out, nil
}

// summariseFirstPaint prints the start-up to first-paint times in Sona's
// JSON logs.
func summariseFirstPaint(files []string) error {
	if len(files) == 0 {
		return errors.New("-firstpaint: give one or more sona.log files")
	}
	paints, err := firstPaints(files...)
	if err != nil {
		return err
	}
	ms := make([]int64, len(paints))
	for i, p := range paints {
		ms[i] = p.Milliseconds()
	}
	slices.Sort(ms)
	fmt.Printf("first paint over %d starts: min %d ms, median %d ms, max %d ms\n",
		len(ms), ms[0], ms[len(ms)/2], ms[len(ms)-1])
	return nil
}
