package library

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// sortOpt is an entry of a Sort select.
type sortOpt struct {
	key   string
	label func() string
}

// listSpec is what a library list's URL may say: its path and the orders it
// knows (search-params.ts's listSearch). The first order is the default.
type listSpec struct {
	path  string
	sorts []sortOpt
	// types: the list has the ?type= filter (albums).
	types bool
	// find: the list has the ?q= find field (songs).
	find bool
	// noDefaultDesc: a URL with a sort and no ?desc is ascending, whatever
	// the sort (songs); else it follows startsDesc.
	noDefaultDesc bool
}

// startsDesc: sorts that read best newest-first start descending.
func startsDesc(sort string) bool {
	return sort == "dateAdded" || sort == "releaseDate" || sort == "lastModified"
}

// listQuery is what the URL says (?sort=…&desc=true&type=singles); an
// unknown value is dropped as v0.2.0's zod schema drops it.
type listQuery struct {
	sort string
	desc bool
	kind string // albums: "", "albums" or "singles"
	// find is the words the list is narrowed to (?q=, songs), as typed: the
	// find field edits it, so it is not trimmed.
	find string
}

// parse reads the page's query. A sort the list does not know falls back to
// the list's default; ?desc is a boolean or dropped.
func (s listSpec) parse(r *ui.Route) listQuery {
	q := listQuery{sort: s.sorts[0].key}
	if v := r.Query("sort"); v != "" {
		for _, o := range s.sorts {
			if o.key == v {
				q.sort = v
			}
		}
	}
	q.desc = !s.noDefaultDesc && startsDesc(q.sort)
	switch r.Query("desc") {
	case "true":
		q.desc = true
	case "false":
		q.desc = false
	}
	if s.types {
		switch t := r.Query("type"); t {
		case "albums", "singles":
			q.kind = t
		}
	}
	if s.find {
		q.find = r.Query("q")
	}
	return q
}

// location is the page's URL for a query: the explicit sort and order, and
// the type and the find words when there are any.
func (s listSpec) location(q listQuery) string {
	v := url.Values{}
	v.Set("sort", q.sort)
	v.Set("desc", map[bool]string{true: "true", false: "false"}[q.desc])
	if q.kind != "" {
		v.Set("type", q.kind)
	}
	if q.find != "" {
		v.Set("q", q.find)
	}
	return s.path + "?" + v.Encode()
}

// key is q as part of a query key: its identity for scroll resets.
func (q listQuery) key() string {
	return q.sort + "/" + map[bool]string{true: "desc", false: "asc"}[q.desc] + "/" + q.kind + "/" + q.find
}

// filtering reports whether the find words narrow the list: it has a word, as
// the search index reads one (a letter or a digit); else it lists everything.
func (q listQuery) filtering() bool {
	return strings.IndexFunc(q.find, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) >= 0
}

var (
	titleSort       = sortOpt{"title", func() string { return i18n.T("Title") }}
	artistSort      = sortOpt{"artist", func() string { return i18n.T("Artist") }}
	albumSort       = sortOpt{"album", func() string { return i18n.T("Album") }}
	dateAddedSort   = sortOpt{"dateAdded", func() string { return i18n.T("Recently added") }}
	releaseDateSort = sortOpt{"releaseDate", func() string { return i18n.T("Release date") }}
	lastModified    = sortOpt{"lastModified", func() string { return i18n.T("Recently updated") }}
)

var (
	albumsSpec    = listSpec{path: albumsPath, types: true, sorts: []sortOpt{dateAddedSort, titleSort, artistSort, releaseDateSort}}
	songsSpec     = listSpec{path: songsPath, noDefaultDesc: true, find: true, sorts: []sortOpt{titleSort, artistSort, albumSort, dateAddedSort}}
	playlistsSpec = listSpec{path: playlistsPath, sorts: []sortOpt{dateAddedSort, lastModified, titleSort}}
)
