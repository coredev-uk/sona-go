package measure

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math/rand/v2"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeApp answers MyGo's automation protocol for an Albums page of 40
// albums in 4 columns: rows 266 DIPs apart, the first at y=300 when the
// page is at its top, and 3000 DIPs of scroll. Every wheel event that moves
// the page draws a frame.
type fakeApp struct {
	mu     sync.Mutex
	offset float64
	frames []Frame
	wheels int
	// noWindowFor answers "windows" with no_window this many times first.
	noWindowFor int
}

const (
	fakeMax  = 3000.0
	fakeRowH = 266.0
	fakeTop  = 300.0
)

func (a *fakeApp) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.frames)
}

func (a *fakeApp) Since(mark int) []Frame {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Frame(nil), a.frames[min(mark, len(a.frames)):]...)
}

func (a *fakeApp) tree() []map[string]any {
	nodes := []map[string]any{
		{"id": "1", "role": "scroll", "bounds": map[string]any{"x": 233, "y": 49, "width": 900, "height": 1000}},
		{"id": "2", "parent": "1", "role": "text", "label": "40 albums", "bounds": map[string]any{"x": 265, "y": 112, "width": 100, "height": 18}},
		// The player bar's play button is not in a scroll container.
		{"id": "3", "role": "button", "key": "play", "label": "Play", "bounds": map[string]any{"x": 500, "y": 1200, "width": 36, "height": 36}},
	}
	for i := range 40 {
		nodes = append(nodes, map[string]any{
			"id": fmt.Sprint(10 + i), "parent": "1", "role": "button", "key": "play", "label": fmt.Sprint("Play ", i),
			"bounds": map[string]any{"x": 265 + 216*(i%4), "y": fakeTop + fakeRowH*float64(i/4) - a.offset, "width": 36, "height": 36},
		})
	}
	return nodes
}

func (a *fakeApp) handle(line []byte) any {
	var req struct {
		ID     json.RawMessage
		Method string
		Params struct {
			Events []struct {
				Type string
				DY   float64
			}
		}
	}
	if err := json.Unmarshal(line, &req); err != nil {
		return map[string]any{"error": map[string]any{"code": "parse_error", "message": err.Error()}}
	}
	res := map[string]any{"id": req.ID}
	a.mu.Lock()
	defer a.mu.Unlock()
	switch req.Method {
	case "windows":
		if a.noWindowFor > 0 {
			a.noWindowFor--
			res["error"] = map[string]any{"code": "no_window", "message": "the app has no window"}
			return res
		}
		res["result"] = map[string]any{"windows": []map[string]any{{"id": 1, "kind": "content", "contentBounds": map[string]any{"x": 0, "y": 0, "width": 1132, "height": 1253}}}}
	case "tree":
		res["result"] = map[string]any{"window": 1, "nodes": a.tree()}
	case "input":
		for _, e := range req.Params.Events {
			a.wheels++
			if next := max(0, min(fakeMax, a.offset+e.DY)); next != a.offset {
				a.offset = next
				a.frames = append(a.frames, Frame{At: time.Now(), Build: 1, Layout: 0.5, Paint: 0.5, Present: 5, Path: "drawn in memory"})
			}
		}
		res["result"] = map[string]any{}
	case "idle":
		res["result"] = map[string]any{}
	case "capture":
		// A window full of noise: every cover is a picture.
		img := image.NewRGBA(image.Rect(0, 0, 1132, 1253))
		rng := rand.New(rand.NewPCG(1, 2))
		for i := range img.Pix {
			img.Pix[i] = uint8(rng.UintN(256))
			if i%4 == 3 {
				img.Pix[i] = 0xff
			}
		}
		var buf bytes.Buffer
		_ = png.Encode(&buf, img)
		res["result"] = map[string]any{"png": buf.Bytes(), "width": 1132, "height": 1253}
	default:
		res["error"] = map[string]any{"code": "unknown_method", "message": req.Method}
	}
	return res
}

func (a *fakeApp) serve(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				sc := bufio.NewScanner(c)
				enc := json.NewEncoder(c)
				for sc.Scan() {
					if enc.Encode(a.handle(sc.Bytes())) != nil {
						return
					}
				}
			}()
		}
	}()
	return path
}

func TestAlbumsScenarioScrollsThePageAndPrintsTheTable(t *testing.T) {
	app := &fakeApp{noWindowFor: 2}
	c, err := Dial(app.serve(t), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	// Draw a first frame so the page has settled when the scenario starts.
	app.mu.Lock()
	app.frames = append(app.frames, Frame{At: time.Now().Add(-time.Second), Path: "drawn in memory"})
	app.mu.Unlock()

	var out, log bytes.Buffer
	err = Albums(Config{
		Client: c, Frames: app, Speed: 30000, Max: 20000, Runs: 2, Idle: 10 * time.Millisecond,
		Interval: 16.67, Quiet: 30 * time.Millisecond, Started: time.Now().Add(-2 * time.Second),
		Out: &out, Log: &log,
	})
	if err != nil {
		t.Fatalf("%v\nout:\n%s\nlog:\n%s", err, &out, &log)
	}
	// The page's end is where the fake clamps it: 3000 DIPs, found from the
	// rows' positions alone.
	want := fmt.Sprintf("the page scrolls %.0f px, %.0f px each way", fakeMax, fakeMax)
	if !bytes.Contains(log.Bytes(), []byte(want)) {
		t.Errorf("log lacks %q:\n%s", want, &log)
	}
	text := out.String()
	for _, sub := range []string{"startup (ms since process start): first frame", "frames: drawn in memory", TableHeader, "| empty (control", "| albums |", "3000 px scrolled each way at 30000 px/s"} {
		if !bytes.Contains([]byte(text), []byte(sub)) {
			t.Errorf("output lacks %q:\n%s", sub, text)
		}
	}
	if bytes.Count(log.Bytes(), []byte("albums run ")) != 2 {
		t.Errorf("want 2 per-pass lines:\n%s", &log)
	}
	// Both passes ended at the top, as the page was found.
	app.mu.Lock()
	defer app.mu.Unlock()
	if app.offset != 0 {
		t.Errorf("offset after the runs = %v, want 0", app.offset)
	}
}

func TestTilesAreThePlayButtonsOfAScrollContainer(t *testing.T) {
	var nodes []Node
	raw := `[
	  {"id":"1","role":"scroll"},
	  {"id":"2","parent":"1","role":"button","key":"play","label":"Play 1989"},
	  {"id":"3","parent":"1","role":"button","key":"other","label":"Sort"},
	  {"id":"4","role":"button","key":"play","label":"Play"}]`
	if err := json.Unmarshal([]byte(raw), &nodes); err != nil {
		t.Fatal(err)
	}
	tiles, page := Tiles(nodes)
	if len(tiles) != 1 || tiles[0].ID != "2" || page.ID != "1" {
		t.Errorf("tiles %+v page %+v: want the one inside the scroll container", tiles, page)
	}
	// The player bar's play button, outside one, is not a tile.
	if tiles, _ := Tiles(nodes[3:]); len(tiles) != 0 {
		t.Errorf("a button outside a scroll container is a tile: %+v", tiles)
	}
}
