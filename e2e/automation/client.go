// Package automation is a client of MyGo's automation plugin
// (github.com/egoist/mygo/plugins/automation, linked into the app by
// -tags probe and listening on $MYGO_AUTOMATION): newline-delimited JSON
// requests over a Unix domain socket that drive a running app through the
// accessibility tree of its windows, its input and its frames. The e2e
// smoke (e2e/) uses it to drive the built app from outside; the protocol is
// documented in MyGo's docs/plugins/automation.md.
package automation

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"slices"
	"strings"
	"sync"
	"time"
)

// Error is a request the app answered with an error: Code is one of the
// plugin's (no_window, no_element, timeout, …).
type Error struct{ Code, Message string }

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Rect is a box in DIPs, relative to the window's content.
type Rect struct{ X, Y, Width, Height float64 }

// Centre is the middle of r.
func (r Rect) Centre() (x, y float64) { return r.X + r.Width/2, r.Y + r.Height/2 }

// Node is an element of a window's tree, what a screen reader reads.
type Node struct {
	ID     string `json:"id"`
	Parent string `json:"parent"`
	Role   string `json:"role"`
	Key    any    `json:"key"` // the element's Key: a string, a number or a bool
	Label  string `json:"label"`
	// Description is the accessible description: the text under a tile's
	// title, which a button or link cannot hold as children.
	Description string   `json:"description"`
	Value       string   `json:"value"`
	Bounds      Rect     `json:"bounds"`
	States      []string `json:"states"`
	Actions     []string `json:"actions"`
}

// Has reports whether the node has the state.
func (n Node) Has(state string) bool { return slices.Contains(n.States, state) }

// Tree is a window's elements in tree order, a parent before its children.
type Tree struct {
	Window int    `json:"window"`
	Focus  string `json:"focus"`
	Nodes  []Node `json:"nodes"`
}

// Find is the first node with the role ("" for any) whose label is exactly
// label.
func (t *Tree) Find(role, label string) (Node, bool) {
	for _, n := range t.Nodes {
		if n.Label == label && (role == "" || n.Role == role) {
			return n, true
		}
	}
	return Node{}, false
}

// Under is the part of the tree below the first node with this role ("" for
// any) and label: its descendants, found by their Parent chain, in tree
// order. It is empty when there is no such node.
func (t *Tree) Under(role, label string) *Tree {
	root, ok := t.Find(role, label)
	out := &Tree{Window: t.Window, Focus: t.Focus}
	if !ok {
		return out
	}
	in := map[string]bool{root.ID: true}
	for _, n := range t.Nodes {
		if in[n.Parent] {
			in[n.ID] = true
			out.Nodes = append(out.Nodes, n)
		}
	}
	return out
}

// Labels are the labels of the nodes with the role ("" for all).
func (t *Tree) Labels(role string) []string {
	var out []string
	for _, n := range t.Nodes {
		if n.Label != "" && (role == "" || n.Role == role) {
			out = append(out, n.Label)
		}
	}
	return out
}

// HasText reports whether a node has this label, value or description.
func (t *Tree) HasText(s string) bool {
	return slices.ContainsFunc(t.Nodes, func(n Node) bool { return n.Label == s || n.Value == s || n.Description == s })
}

// Window is an open window.
type Window struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Kind          string `json:"kind"`
	Bounds        Rect   `json:"bounds"`
	ContentBounds Rect   `json:"contentBounds"`
	Focused       bool   `json:"focused"`
	Visible       bool   `json:"visible"`
}

// Event is an input event of the "input" request: type, and the fields it
// takes (x, y, key, text, button, clicks, mods).
type Event map[string]any

// Click is a left click at x, y.
func Click(x, y float64) Event { return Event{"type": "click", "x": x, "y": y} }

// Key presses and releases a key and its modifiers as accelerators write
// them ("Space", "Shift+Tab", "Ctrl+B").
func Key(key string) Event { return Event{"type": "key", "key": key} }

// Client is a connection to the app's automation socket. It is safe for use
// by one goroutine at a time.
type Client struct {
	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
	id   int
}

// Dial connects to the socket at path, retrying until it appears or wait
// has passed (the app creates it while starting).
func Dial(path string, wait time.Duration) (*Client, error) {
	var err error
	for deadline := time.Now().Add(wait); ; time.Sleep(50 * time.Millisecond) {
		var conn net.Conn
		if conn, err = net.Dial("unix", path); err == nil {
			r := bufio.NewReaderSize(conn, 1<<20)
			return &Client{conn: conn, r: r}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("automation: %s did not appear in %v: %w", path, wait, err)
		}
	}
}

// Close ends the connection.
func (c *Client) Close() error { return c.conn.Close() }

// Call sends a request and decodes its result into result (nil to ignore
// it). An error answer is an *Error.
func (c *Client) Call(method string, params, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.id++
	line, err := json.Marshal(map[string]any{"id": c.id, "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return err
	}
	answer, err := c.r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("automation: %s: %w", method, err)
	}
	var res struct {
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		return fmt.Errorf("automation: %s: %w", method, err)
	}
	if res.Error != nil {
		return res.Error
	}
	if result != nil {
		return json.Unmarshal(res.Result, result)
	}
	return nil
}

// Windows lists the open windows.
func (c *Client) Windows() ([]Window, error) {
	var res struct{ Windows []Window }
	err := c.Call("windows", nil, &res)
	return res.Windows, err
}

// Tree reads the elements of the default window.
func (c *Client) Tree() (*Tree, error) {
	var t Tree
	if err := c.Call("tree", nil, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Act performs an action ("press", "focus", "setValue", …) on an element.
func (c *Client) Act(id, action string) error {
	return c.Call("act", map[string]any{"id": id, "action": action}, nil)
}

// Input sends events to the default window, in order.
func (c *Client) Input(events ...Event) error {
	return c.Call("input", map[string]any{"events": events}, nil)
}

// Idle waits until the window asks for no more frames, at most timeout.
func (c *Client) Idle(timeout time.Duration) error {
	return c.Call("idle", map[string]any{"timeout": timeout.Milliseconds()}, nil)
}

// Capture is the default window as an image.
func (c *Client) Capture() (image.Image, []byte, error) {
	var res struct {
		PNG []byte `json:"png"`
	}
	if err := c.Call("capture", nil, &res); err != nil {
		return nil, nil, err
	}
	img, err := png.Decode(bytes.NewReader(res.PNG))
	return img, res.PNG, err
}

// ErrTimeout is Wait's failure.
var ErrTimeout = errors.New("automation: timed out")

// Wait reads the tree again and again (waiting for the window to settle
// between reads) until ok holds, and returns that tree. After timeout it
// fails with ErrTimeout and the labels of the last tree.
func (c *Client) Wait(timeout time.Duration, what string, ok func(*Tree) bool) (*Tree, error) {
	var last *Tree
	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		_ = c.Idle(2 * time.Second) // an animation that never settles is not an error here
		t, err := c.Tree()
		if err != nil {
			var e *Error
			if errors.As(err, &e) && e.Code == "no_window" {
				continue // the window is not there yet
			}
			return nil, err
		}
		if last = t; ok(t) {
			return t, nil
		}
	}
	labels := "no tree read"
	if last != nil {
		labels = strings.Join(last.Labels(""), " | ")
	}
	return last, fmt.Errorf("%w waiting for %s; the window shows: %s", ErrTimeout, what, labels)
}

// Press presses the first button labelled label, waiting for it to appear,
// with the pointer: it moves onto the button (so what shows only while
// hovered is there), lets the window settle and clicks. act/press would not
// do: a control that is transparent until hovered does not take it.
func (c *Client) Press(timeout time.Duration, label string) error {
	t, err := c.Wait(timeout, "button "+label, func(t *Tree) bool { _, ok := t.Find("button", label); return ok })
	if err != nil {
		return err
	}
	n, _ := t.Find("button", label)
	x, y := n.Bounds.Centre()
	if err := c.Input(Event{"type": "pointerMove", "x": x, "y": y}); err != nil {
		return err
	}
	_ = c.Idle(2 * time.Second)
	return c.Input(Click(x, y))
}
