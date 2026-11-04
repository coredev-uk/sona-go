package search

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Search scopes: where a search looks.
const (
	scopeCatalog = "catalog"
	scopeLibrary = "library"
)

// cleanTerm is a search term as the route keeps it: trimmed; blank is none.
func cleanTerm(q string) string { return strings.TrimSpace(q) }

// cleanScope is the scope the route names: "library" or the default,
// "catalog" (an unknown value is dropped).
func cleanScope(scope string) string {
	if scope == scopeLibrary {
		return scopeLibrary
	}
	return scopeCatalog
}

// searchParams reads /search?q=…&scope=… from a location (the router's
// Location()): q trimmed, "" when blank; scope "catalog" (the default) or
// "library". Both are empty/default when the location is another page.
func searchParams(loc string) (q, scope string) {
	path, query, _ := strings.Cut(loc, "?")
	if path != searchPath {
		return "", scopeCatalog
	}
	v, _ := url.ParseQuery(query)
	return cleanTerm(v.Get("q")), cleanScope(v.Get("scope"))
}

// routeParams is searchParams over a route (the page's own query).
func routeParams(r *ui.Route) (q, scope string) {
	return cleanTerm(r.Query("q")), cleanScope(r.Query("scope"))
}

// searchLocation is the URL of a search: the scope is written only for the
// library (the default is left out), a blank term is dropped.
func searchLocation(q, scope string) string {
	v := url.Values{}
	if q = cleanTerm(q); q != "" {
		v.Set("q", q)
	}
	if scope == scopeLibrary {
		v.Set("scope", scopeLibrary)
	}
	if len(v) == 0 {
		return searchPath
	}
	return searchPath + "?" + v.Encode()
}

// groupLocation is a group's "see all" page for a term.
func groupLocation(group, q string) string {
	loc := "/search/" + url.PathEscape(group)
	if q = cleanTerm(q); q != "" {
		loc += "?" + url.Values{"q": {q}}.Encode()
	}
	return loc
}
