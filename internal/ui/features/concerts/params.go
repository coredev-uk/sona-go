package concerts

import (
	"net/url"
	"regexp"

	"github.com/egoist/mygo/ui"
)

// concertsPath is the Concerts page's route.
const concertsPath = "/concerts"

// The date filter's choices (v0.2.0's whenValues); "dates" is a range the
// user types in.
const (
	whenWeek    = "week"
	whenWeekend = "weekend"
	whenMonth   = "month"
	whenDates   = "dates"
)

// params is what /concerts?when=…&from=…&to=…&genre=…&yours=true says
// (v0.2.0's concertsSearch): the date filter (a preset, or "dates" with
// from and to as "2026-11-14"), an Apple genre ID, and only the user's own
// artists. Anything unreadable is dropped, as zod's catch did.
type params struct {
	when, from, to, genre string
	yours                 bool
}

var (
	isoDayPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	digitsPattern = regexp.MustCompile(`^\d+$`)
)

// paramsFrom reads the parameters from a query.
func paramsFrom(q url.Values) params {
	var p params
	switch w := q.Get("when"); w {
	case whenWeek, whenWeekend, whenMonth, whenDates:
		p.when = w
	}
	if v := q.Get("from"); isoDayPattern.MatchString(v) {
		p.from = v
	}
	if v := q.Get("to"); isoDayPattern.MatchString(v) {
		p.to = v
	}
	if v := q.Get("genre"); digitsPattern.MatchString(v) {
		p.genre = v
	}
	p.yours = q.Get("yours") == "true"
	return p
}

// paramsOf is the page's parameters from its route.
func paramsOf(r *ui.Route) params {
	q := url.Values{}
	for _, k := range []string{"when", "from", "to", "genre", "yours"} {
		if v := r.Query(k); v != "" {
			q.Set(k, v)
		}
	}
	return paramsFrom(q)
}

// location is the page's URL for the parameters: only what is set.
func (p params) location() string {
	q := url.Values{}
	if p.when != "" {
		q.Set("when", p.when)
	}
	if p.from != "" {
		q.Set("from", p.from)
	}
	if p.to != "" {
		q.Set("to", p.to)
	}
	if p.genre != "" {
		q.Set("genre", p.genre)
	}
	if p.yours {
		q.Set("yours", "true")
	}
	if len(q) == 0 {
		return concertsPath
	}
	return concertsPath + "?" + q.Encode()
}
