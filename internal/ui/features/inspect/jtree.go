package inspect

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// jkind is what a JSON value is.
type jkind uint8

const (
	jObject jkind = iota
	jArray
	jString
	jNumber
	jBool
	jNull
)

// jnode is one JSON value of a payload, with the key it has in its parent
// and its path from the root (`attributes.artwork.url`, `items[0].id`).
type jnode struct {
	key    string
	path   string
	kind   jkind
	text   string // a scalar as written: a string unquoted, a number as in the payload
	kids   []*jnode
	parent *jnode
	depth  int
}

// container reports whether n holds other values.
func (n *jnode) container() bool { return n.kind == jObject || n.kind == jArray }

// openByDefault is how many levels a tree is open when it first shows: the
// containers of the top two levels show their contents and everything below
// folds, so a payload reads at a glance and a large one is never built whole
// (docs/architecture.md §8).
const openByDefault = 3

// maxPayload is the largest payload parsed for a tree: Apple's pages are a
// few hundred kilobytes, and anything larger is shown as text only by the
// caller.
const maxPayload = 8 << 20

// parseJSON reads text into a tree, keeping the order of an object's keys; nil
// when it is not one JSON value or is too large.
func parseJSON(text string) *jnode {
	if text == "" || len(text) > maxPayload {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	root, err := decodeValue(dec, nil, "", "")
	if err != nil {
		return nil
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil
	}
	return root
}

func decodeValue(dec *json.Decoder, parent *jnode, key, path string) (*jnode, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	n := &jnode{key: key, path: path, parent: parent}
	if parent != nil {
		n.depth = parent.depth + 1
	}
	switch t := tok.(type) {
	case json.Delim:
		if t == '{' {
			n.kind = jObject
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				kid, err := decodeValue(dec, n, k, joinPath(path, k))
				if err != nil {
					return nil, err
				}
				n.kids = append(n.kids, kid)
			}
		} else {
			n.kind = jArray
			for i := 0; dec.More(); i++ {
				idx := "[" + strconv.Itoa(i) + "]"
				kid, err := decodeValue(dec, n, idx, path+idx)
				if err != nil {
					return nil, err
				}
				n.kids = append(n.kids, kid)
			}
		}
		if _, err := dec.Token(); err != nil { // the closing delimiter
			return nil, err
		}
	case string:
		n.kind, n.text = jString, t
	case json.Number:
		n.kind, n.text = jNumber, t.String()
	case bool:
		n.kind, n.text = jBool, strconv.FormatBool(t)
	case nil:
		n.kind, n.text = jNull, "null"
	}
	return n, nil
}

// joinPath is the path of key under path: dotted, with a bracketed index
// kept to the array it belongs to.
func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// summary is what a folded container says of itself: "12 keys", "3 items",
// or the empty braces.
func (n *jnode) summary() string {
	switch {
	case n.kind == jObject && len(n.kids) == 0:
		return "{}"
	case n.kind == jArray && len(n.kids) == 0:
		return "[]"
	case n.kind == jObject && len(n.kids) == 1:
		return "1 key"
	case n.kind == jObject:
		return strconv.Itoa(len(n.kids)) + " keys"
	case len(n.kids) == 1:
		return "1 item"
	}
	return strconv.Itoa(len(n.kids)) + " items"
}

// value is the scalar as the tree shows it: a string in quotes.
func (n *jnode) value() string {
	if n.kind == jString {
		return strconv.Quote(n.text)
	}
	return n.text
}

// jtree is a payload with the state of its view: what is open, the filter,
// the chosen row.
type jtree struct {
	root *jnode
	// state is MyGo's outline: the rows in view and the items open.
	state ui.OutlineState[*jnode]
	// chosen is the row chosen, -1 for none.
	chosen int
	// query is the filter field's text, and applied the text the filter
	// last ran with: a change recomputes keep and the open items.
	query, applied string
	// keep is the nodes the filter shows (matches and their ancestors); nil
	// while no filter applies. matches is how many nodes match, shown is how
	// many of them are kept (at most limit).
	keep           map[*jnode]bool
	matches, shown int
	limit          int
}

// matchLimit is how many matches a filter shows before "Show more".
const matchLimit = 400

func newJTree(text string) *jtree {
	root := parseJSON(text)
	if root == nil {
		return nil
	}
	t := &jtree{root: root, chosen: -1, limit: matchLimit}
	t.state.List.Selected = &t.chosen
	t.openDefault()
	return t
}

// openDefault opens the containers of the first levels and folds the rest.
func (t *jtree) openDefault() {
	t.state.Open = ui.Selection[*jnode]{}
	var walk func(n *jnode)
	walk = func(n *jnode) {
		if n.container() && len(n.kids) > 0 && n.depth < openByDefault {
			t.state.Open.Add(n)
			for _, k := range n.kids {
				walk(k)
			}
		}
	}
	walk(t.root)
}

// roots are the items at the top of the outline: the root's own children, so
// the first rows are the payload's keys (a scalar root is its own row).
func (t *jtree) roots() []*jnode {
	if !t.root.container() {
		return []*jnode{t.root}
	}
	return t.visible(t.root.kids)
}

// children is the outline's children of n: nil for a value that holds none.
func (t *jtree) children(n *jnode) []*jnode {
	if !n.container() || len(n.kids) == 0 {
		return nil
	}
	return t.visible(n.kids)
}

// visible is kids, or the ones the filter keeps.
func (t *jtree) visible(kids []*jnode) []*jnode {
	if t.keep == nil {
		return kids
	}
	out := make([]*jnode, 0, len(kids))
	for _, k := range kids {
		if t.keep[k] {
			out = append(out, k)
		}
	}
	return out
}

// setFilter applies the filter once its text changed: nodes whose key or
// value holds every word (folded to lower case) are matches, they and their
// ancestors are kept and every kept container opens. No text clears it and
// restores the first levels.
func (t *jtree) setFilter() {
	if t.query == t.applied {
		return
	}
	t.applied = t.query
	t.limit = matchLimit
	t.refilter()
}

func (t *jtree) refilter() {
	words := strings.Fields(strings.ToLower(t.query))
	if len(words) == 0 {
		t.keep, t.matches, t.shown = nil, 0, 0
		t.openDefault()
		return
	}
	keep := map[*jnode]bool{}
	t.matches, t.shown = 0, 0
	var walk func(n *jnode)
	walk = func(n *jnode) {
		for _, k := range n.kids {
			if matchesAll(k, words) {
				t.matches++
				if t.shown < t.limit {
					t.shown++
					for a := k; a != nil && a != t.root && !keep[a]; a = a.parent {
						keep[a] = true
					}
				}
			}
			walk(k)
		}
	}
	if matchesAll(t.root, words) && !t.root.container() {
		t.matches, t.shown = 1, 1
		keep[t.root] = true
	}
	walk(t.root)
	t.keep = keep
	t.state.Open = ui.Selection[*jnode]{}
	for n := range keep {
		if n.container() {
			t.state.Open.Add(n)
		}
	}
}

// more raises the limit of matches shown.
func (t *jtree) more() {
	t.limit += matchLimit
	t.refilter()
}

// matchesAll reports whether the key and value of n hold every word.
func matchesAll(n *jnode, words []string) bool {
	hay := strings.ToLower(n.key + " " + n.text)
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// noMatches is a filter that keeps nothing.
func (t *jtree) noMatches() bool { return t.keep != nil && t.matches == 0 }

// hasMore is a filter with matches left out.
func (t *jtree) hasMore() bool { return t.keep != nil && t.matches > t.shown }

// chosenNode is the node of the chosen row, nil for none.
func (t *jtree) chosenNode() *jnode {
	if t.chosen < 0 || t.chosen >= t.state.Rows() {
		return nil
	}
	return t.state.Item(t.chosen)
}

// copyValue is what Copy value puts on the clipboard: a scalar as written (a
// string without its quotes), a container as JSON.
func (n *jnode) copyValue() string {
	if !n.container() {
		return n.text
	}
	b, err := json.MarshalIndent(n.any(), "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// any is n as the values encoding/json makes.
func (n *jnode) any() any {
	switch n.kind {
	case jObject:
		return pairs(n.kids)
	case jArray:
		out := make([]any, len(n.kids))
		for i, k := range n.kids {
			out[i] = k.any()
		}
		return out
	case jString:
		return n.text
	case jNumber:
		return json.Number(n.text)
	case jBool:
		return n.text == "true"
	}
	return nil
}

// pairs is an object's values, marshalled in the order the payload had.
type pairs []*jnode

// MarshalJSON writes the pairs in order.
func (p pairs) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range p {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(k.key)
		val, err := json.Marshal(k.any())
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}
