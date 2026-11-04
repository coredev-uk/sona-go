package measure

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Client speaks MyGo's automation protocol (newline-delimited JSON over a
// Unix socket, plugins/automation): the app's accessibility tree, wheel
// and key input, idle waits and captures. One request at a time.
type Client struct {
	// Timeout is how long a request may wait for its answer (a minute
	// when zero): an app that never answers ends the probe with an error.
	Timeout time.Duration

	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
	id   int
}

// Error is a request's failure as the app reports it.
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Dial connects to the automation socket at path, retrying until the app
// has created it or timeout passes.
func Dial(path string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.Dial("unix", path)
		if err == nil {
			return &Client{conn: conn, r: bufio.NewReaderSize(conn, 1<<20)}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("automation socket %s: %w (is this a probe build started with MYGO_AUTOMATION=%s?)", path, err, path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call performs a request and decodes its result into out (nil to ignore
// it).
func (c *Client) Call(method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	timeout := c.Timeout
	if timeout == 0 {
		timeout = time.Minute
	}
	if err := c.conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	line, err := json.Marshal(map[string]any{"id": c.id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return err
	}
	answer, err := c.r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	var res struct {
		Result json.RawMessage
		Error  *Error
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if res.Error != nil {
		return fmt.Errorf("%s: %w", method, res.Error)
	}
	if out != nil {
		return json.Unmarshal(res.Result, out)
	}
	return nil
}

// Rect is a box in DIPs.
type Rect struct{ X, Y, Width, Height float64 }

// Node is an element of the accessibility tree.
type Node struct {
	ID     string
	Parent string
	Role   string
	Key    json.RawMessage
	Label  string
	Value  string
	Bounds Rect
	States []string
}

// HasKey tells whether the node's key is the string k.
func (n Node) HasKey(k string) bool {
	var s string
	return json.Unmarshal(n.Key, &s) == nil && s == k
}

// Tree is the elements of the window's last frame.
func (c *Client) Tree() ([]Node, error) {
	var res struct{ Nodes []Node }
	if err := c.Call("tree", nil, &res); err != nil {
		return nil, err
	}
	return res.Nodes, nil
}

// WaitWindow waits until the app has a window of native UI; the plugin
// answers before Sona's own window exists.
func (c *Client) WaitWindow(timeout time.Duration) (Rect, error) {
	deadline := time.Now().Add(timeout)
	for {
		var res struct {
			Windows []struct {
				Kind          string
				ContentBounds Rect
			}
		}
		err := c.Call("windows", nil, &res)
		if err == nil {
			for _, w := range res.Windows {
				if w.Kind == "content" {
					return w.ContentBounds, nil
				}
			}
		}
		var e *Error
		if err != nil && (!errors.As(err, &e) || e.Code != "no_window") {
			return Rect{}, err
		}
		if time.Now().After(deadline) {
			return Rect{}, errors.New("the app opened no window")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Wheel scrolls by dy DIPs (positive: down) at the point x, y of the
// window, as a touchpad does.
func (c *Client) Wheel(x, y, dy float64) error {
	return c.Call("input", map[string]any{"events": []map[string]any{
		{"type": "wheel", "x": x, "y": y, "dx": 0, "dy": dy, "precise": true},
	}}, nil)
}

// PointerMove moves the pointer to x, y of the window.
func (c *Client) PointerMove(x, y float64) error {
	return c.Call("input", map[string]any{"events": []map[string]any{
		{"type": "pointerMove", "x": x, "y": y},
	}}, nil)
}

// Idle waits until the window asks for no frame it has not drawn.
func (c *Client) Idle(timeout time.Duration) error {
	return c.Call("idle", map[string]any{"timeout": timeout.Milliseconds()}, nil)
}

// Capture is the window as PNG bytes.
func (c *Client) Capture() ([]byte, error) {
	var res struct{ PNG []byte }
	if err := c.Call("capture", nil, &res); err != nil {
		return nil, err
	}
	return res.PNG, nil
}

// Tiles are the album tiles of the page in the tree: the buttons keyed
// "play" inside a scroll container (the player bar's is outside one), and
// that container, the page.
func Tiles(nodes []Node) (tiles []Node, page Node) {
	scrolls := map[string]Node{}
	for _, n := range nodes {
		if n.Role == "scroll" {
			scrolls[n.ID] = n
		}
	}
	for _, n := range nodes {
		if n.Role == "button" && n.HasKey("play") {
			if p, ok := scrolls[n.Parent]; ok {
				tiles, page = append(tiles, n), p
			}
		}
	}
	return tiles, page
}
