package inspect

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// rowKind is how a ledger value is drawn.
type rowKind uint8

const (
	rowText   rowKind = iota
	rowNumber         // tabular figures
	rowBool
	rowNull
	rowColour // a swatch, then the hex
	rowLink   // opens on a click
	rowArt    // an artwork URL: a link that is also copied
)

// ledgerRow is one label/value row of the Overview or Library ledger.
type ledgerRow struct {
	// label is the field's path (`traits.lossless`).
	label string
	// value is what the row shows; copy is what Copy puts on the clipboard
	// (the value as the payload has it); note is a muted second reading of the
	// value (a duration's raw milliseconds, a date's raw stamp).
	value, copy, note string
	kind              rowKind
	// colour is the swatch of a rowColour.
	colour ui.Color
	// url is where a rowLink and a rowArt open.
	url string
	// group is the section of the ledger the row is in; a change of group
	// is a wider gap.
	group ledgerGroup
}

// ledgerGroup orders the rows: what the item is, what it says, its flags and
// numbers, its dates, durations, artwork and links.
type ledgerGroup uint8

const (
	groupIdentity ledgerGroup = iota
	groupText
	groupFlags
	groupDates
	groupDurations
	groupArtwork
	groupLinks
)

var (
	hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// idKey: a key that names an ID or the item's origin.
	idKey = regexp.MustCompile(`(?i)(^|\.)(id|source|kind|type|[a-z]*id)$`)
	// durationKey: a length of time in milliseconds.
	durationKey = regexp.MustCompile(`(?i)duration[a-z]*ms$`)
	// dateKey: a Unix-millisecond stamp or a date.
	dateKey = regexp.MustCompile(`(?i)(at|added|modified|date|time[a-z]*ms|airtimems|fetched[a-z]*)$`)
	// nullable is a database/sql null type's field, the pair its value is
	// written as.
	nullable = map[string]bool{"String": true, "Int64": true, "Int32": true, "Int16": true, "Float64": true, "Bool": true, "Time": true, "Byte": true}
)

// ledgerOf flattens a payload into rows in a sensible order: identity, text,
// flags and numbers, dates, durations, artwork, links; the payload's own
// order within a group. art, when it has a template, replaces the item's own
// art fields (Go-served paths) by Apple's template and its URL at each size.
func ledgerOf(root *jnode, art services.InspectArt) []ledgerRow {
	if root == nil {
		return nil
	}
	var rows []ledgerRow
	hasArt := art.Template != ""
	var walk func(n *jnode)
	walk = func(n *jnode) {
		switch {
		case n.container() && nullPair(n):
			rows = append(rows, rowOf(n.path, n.kids[0], true))
		case n.container() && len(n.kids) > 0:
			if hasArt && n.path == "art" {
				return
			}
			for _, k := range n.kids {
				walk(k)
			}
		default:
			if n != root || !n.container() {
				rows = append(rows, rowOf(n.path, n, false))
			}
		}
	}
	walk(root)
	if hasArt {
		rows = append(rows, artRows(art)...)
	}
	slices.SortStableFunc(rows, func(a, b ledgerRow) int { return int(a.group) - int(b.group) })
	return rows
}

// nullPair reports whether n is how database/sql's NullString and its kin
// marshal: {"String": …, "Valid": …}.
func nullPair(n *jnode) bool {
	if n.kind != jObject || len(n.kids) != 2 || n.kids[1].key != "Valid" || n.kids[1].kind != jBool {
		return false
	}
	return nullable[n.kids[0].key]
}

// rowOf is the row of one value. collapsed marks a null pair read as its
// value: Valid false is a null.
func rowOf(path string, n *jnode, collapsed bool) ledgerRow {
	if collapsed {
		valid := n.parent.kids[1].text == "true"
		if !valid {
			return ledgerRow{label: path, value: "null", copy: "", kind: rowNull, group: groupOf(path, jNull, "")}
		}
	}
	r := ledgerRow{label: path, value: n.text, copy: n.text, group: groupOf(path, n.kind, n.text)}
	switch n.kind {
	case jString:
		switch {
		case hexColour.MatchString(n.text):
			r.kind, r.colour = rowColour, ui.Hex(n.text)
		case isWebLink(n.text):
			r.kind, r.url = rowLink, n.text
		case n.text == "":
			r.value, r.kind = "empty", rowNull
		}
	case jNumber:
		r.kind = rowNumber
		v, err := strconv.ParseInt(n.text, 10, 64)
		switch {
		case err != nil:
		case durationKey.MatchString(path):
			r.value, r.note = clock(v), n.text+" ms"
		case isStamp(path):
			r.value, r.note = stamp(v), n.text
		}
	case jBool:
		r.kind = rowBool
	case jNull:
		r.kind = rowNull
	default:
		r.value, r.kind = n.summary(), rowNull
	}
	return r
}

// isStamp: a key that holds a Unix-millisecond time.
func isStamp(path string) bool {
	k := strings.ToLower(last(path))
	return strings.HasSuffix(k, "at") || k == "dateadded" || k == "lastmodified" || k == "airtimems" ||
		strings.HasSuffix(k, "added") || strings.HasSuffix(k, "modified")
}

// groupOf decides the section of a row from its key and value.
func groupOf(path string, kind jkind, text string) ledgerGroup {
	key := last(path)
	switch {
	case strings.HasPrefix(path, "art.") || strings.HasPrefix(path, "motion") || strings.HasSuffix(strings.ToLower(key), "art"):
		return groupArtwork
	case kind == jString && isWebLink(text), strings.EqualFold(key, "route"), strings.EqualFold(key, "url"):
		return groupLinks
	case kind == jNumber && durationKey.MatchString(path):
		return groupDurations
	case (kind == jNumber && isStamp(path)) || (kind == jString && dateKey.MatchString(key) && looksLikeDate(text)):
		return groupDates
	case idKey.MatchString(path):
		return groupIdentity
	case kind == jBool, kind == jNumber, strings.HasPrefix(path, "traits."):
		return groupFlags
	}
	return groupText
}

// looksLikeDate: YYYY, YYYY-MM or YYYY-MM-DD.
func looksLikeDate(s string) bool {
	if len(s) < 4 || len(s) > 10 {
		return false
	}
	for i, r := range s {
		if (i == 4 || i == 7) && r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func last(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}

// isWebLink: an http or https address.
func isWebLink(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// clock is a length in milliseconds as m:ss (h:mm:ss from an hour).
func clock(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	s := int64(d.Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s%3600/60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// stamp is a Unix-millisecond time in the local zone ("not set" for 0).
func stamp(ms int64) string {
	if ms == 0 {
		return i18n.T("not set")
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04 MST")
}

// artRows are the artwork rows an item with a template has: the template and
// its URL at each size Sona shows, each size beside the Go-served path it
// stands for. Apple's URLs open on a click.
func artRows(art services.InspectArt) []ledgerRow {
	rows := []ledgerRow{{label: "art.template", value: art.Template, copy: art.Template, kind: rowText, group: groupArtwork}}
	for _, sz := range art.Sizes {
		r := ledgerRow{
			label: fmt.Sprintf("art.%s · %d×%d", sz.Name, sz.Width, sz.Height), value: sz.URL, copy: sz.URL,
			note: sz.Local, kind: rowArt, url: sz.URL, group: groupArtwork,
		}
		if sz.URL == "" {
			r.kind, r.value = rowNull, "not on Apple's artwork CDN"
		}
		rows = append(rows, r)
	}
	return rows
}
