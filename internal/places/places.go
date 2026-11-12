// Package places is Sona's own list of cities for choosing where to look
// for concerts: names, countries and coordinates, searched on this
// computer. Apple's web player finds places with MapKit JS, a separate
// service with its own token; Sona uses neither, so a chosen city never
// leaves the computer except as the four-character geohash of its area in
// Apple's concerts requests.
package places

import (
	_ "embed"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

//go:embed cities.tsv
var citiesTSV string

// fallbackID is the default for a storefront whose country has no city in
// the list.
const fallbackID = "gb-london"

// City is one place in the list.
type City struct {
	ID        string  // "gb-london": country, then the folded name
	Name      string  // English name as people write it
	Country   string  // ISO 3166-1 alpha-2, lower case: also the storefront
	Latitude  float64 // degrees north
	Longitude float64 // degrees east
	words     []string
}

// Geohash is the city's concerts area (ConcertsPrecision characters).
func (c City) Geohash() string { return Geohash(c.Latitude, c.Longitude, ConcertsPrecision) }

type index struct {
	all       []City
	byID      map[string]int
	firstOf   map[string]int // country → its first (main) city
	countries map[string]bool
}

var load = sync.OnceValue(func() *index {
	ix := &index{byID: map[string]int{}, firstOf: map[string]int{}, countries: map[string]bool{}}
	for line := range strings.SplitSeq(citiesTSV, "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 {
			continue
		}
		lat, err1 := strconv.ParseFloat(f[3], 64)
		lon, err2 := strconv.ParseFloat(f[4], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		c := City{ID: f[0], Name: f[1], Country: f[2], Latitude: lat, Longitude: lon}
		c.words = fold(f[1])
		if len(f) > 5 {
			c.words = append(c.words, fold(f[5])...)
		}
		ix.byID[c.ID] = len(ix.all)
		if _, ok := ix.firstOf[c.Country]; !ok {
			ix.firstOf[c.Country] = len(ix.all)
		}
		ix.countries[c.Country] = true
		ix.all = append(ix.all, c)
	}
	return ix
})

// ByID returns the city with the given ID.
func ByID(id string) (City, bool) {
	ix := load()
	i, ok := ix.byID[id]
	if !ok {
		return City{}, false
	}
	return ix.all[i], true
}

// Default is the city shown before the user chooses one: the main city of
// the storefront's country, else London.
func Default(storefront string) City {
	ix := load()
	if i, ok := ix.firstOf[strings.ToLower(storefront)]; ok {
		return ix.all[i]
	}
	c, _ := ByID(fallbackID)
	return c
}

// worldCities are well-known cities offered after the user's own country's.
var worldCities = []string{
	"us-new-york", "gb-london", "fr-paris", "de-berlin", "jp-tokyo", "au-sydney",
	"ca-toronto", "us-los-angeles", "kr-seoul", "br-sao-paulo", "mx-mexico-city", "es-madrid",
}

// Suggested lists cities to pick from before anything is typed: those of
// the storefront's country, then well-known cities elsewhere.
func Suggested(storefront string, limit int) []City {
	ix := load()
	cc := strings.ToLower(storefront)
	var out []City
	for _, c := range ix.all {
		if c.Country == cc && len(out) < limit {
			out = append(out, c)
		}
	}
	for _, id := range worldCities {
		if len(out) >= limit {
			break
		}
		if c, ok := ByID(id); ok && c.Country != cc {
			out = append(out, c)
		}
	}
	return out
}

// Search finds cities by name, in the user's storefront first. Every word
// of the query must start a word of the city's name, its other names or
// its country (a code or a common English name: "paris fr", "london uk");
// accents and case are ignored. A name that starts with the query comes
// before one that only has a word that does.
func Search(query, storefront string, limit int) []City {
	words := fold(query)
	if len(words) == 0 || limit <= 0 {
		return nil
	}
	type hit struct {
		c     City
		score int
	}
	var hits []hit
	cc := strings.ToLower(storefront)
	for _, c := range load().all {
		score, ok := match(c, words)
		if !ok {
			continue
		}
		if c.Country == cc {
			score -= 2
		}
		hits = append(hits, hit{c, score})
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.score - b.score })
	out := make([]City, 0, min(limit, len(hits)))
	for _, h := range hits[:min(limit, len(hits))] {
		out = append(out, h.c)
	}
	return out
}

// match reports whether every query word starts a word of c, and how well:
// lower is better (0: the name itself starts with the first word).
func match(c City, query []string) (int, bool) {
	// words starts with the name's own words (see load).
	country := countryWords[c.Country]
	score := 3
	for i, q := range query {
		switch {
		case i == 0 && len(c.words) > 0 && strings.HasPrefix(c.words[0], q):
			score = 0
		case slices.ContainsFunc(c.words, func(w string) bool { return strings.HasPrefix(w, q) }):
			score = min(score, 1)
		case c.Country == q || slices.ContainsFunc(country, func(w string) bool { return strings.HasPrefix(w, q) }):
			// The country narrows the search; it does not rank.
		default:
			return 0, false
		}
	}
	return score, true
}

var accents = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// fold lower-cases s, removes accents and splits it into words (letters
// and digits; other scripts stay as they are).
func fold(s string) []string {
	out, _, err := transform.String(accents, strings.ToLower(s))
	if err != nil {
		out = strings.ToLower(s)
	}
	return strings.FieldsFunc(out, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// countryWords are the English names a country is searched by.
var countryWords = map[string][]string{
	"us": {"usa", "united", "states", "america"}, "gb": {"uk", "united", "kingdom", "britain", "england", "scotland", "wales", "northern", "ireland"},
	"ie": {"ireland"}, "ca": {"canada"}, "mx": {"mexico"}, "fr": {"france"}, "de": {"germany", "deutschland"},
	"at": {"austria"}, "ch": {"switzerland"}, "nl": {"netherlands", "holland"}, "be": {"belgium"}, "lu": {"luxembourg"},
	"es": {"spain"}, "pt": {"portugal"}, "it": {"italy"}, "gr": {"greece"}, "cy": {"cyprus"}, "mt": {"malta"},
	"dk": {"denmark"}, "se": {"sweden"}, "no": {"norway"}, "fi": {"finland"}, "is": {"iceland"},
	"ee": {"estonia"}, "lv": {"latvia"}, "lt": {"lithuania"}, "pl": {"poland"}, "cz": {"czechia", "czech", "republic"},
	"sk": {"slovakia"}, "hu": {"hungary"}, "ro": {"romania"}, "bg": {"bulgaria"}, "hr": {"croatia"}, "rs": {"serbia"},
	"si": {"slovenia"}, "ba": {"bosnia"}, "al": {"albania"}, "mk": {"macedonia"}, "me": {"montenegro"}, "ua": {"ukraine"},
	"tr": {"turkey", "türkiye", "turkiye"}, "il": {"israel"}, "ae": {"uae", "emirates"}, "sa": {"saudi", "arabia"},
	"qa": {"qatar"}, "kw": {"kuwait"}, "bh": {"bahrain"}, "om": {"oman"}, "jo": {"jordan"}, "lb": {"lebanon"},
	"eg": {"egypt"}, "za": {"south", "africa"}, "ng": {"nigeria"}, "ke": {"kenya"}, "gh": {"ghana"}, "ma": {"morocco"},
	"tn": {"tunisia"}, "tz": {"tanzania"}, "ug": {"uganda"}, "sn": {"senegal"}, "dz": {"algeria"}, "et": {"ethiopia"},
	"in": {"india"}, "pk": {"pakistan"}, "bd": {"bangladesh"}, "lk": {"sri", "lanka"}, "np": {"nepal"},
	"jp": {"japan"}, "kr": {"korea", "south"}, "cn": {"china"}, "hk": {"hong", "kong"}, "mo": {"macao", "macau"},
	"tw": {"taiwan"}, "sg": {"singapore"}, "my": {"malaysia"}, "th": {"thailand"}, "vn": {"vietnam"},
	"ph": {"philippines"}, "id": {"indonesia"}, "kh": {"cambodia"}, "mm": {"myanmar", "burma"}, "mn": {"mongolia"},
	"au": {"australia"}, "nz": {"zealand"}, "fj": {"fiji"}, "br": {"brazil", "brasil"}, "ar": {"argentina"},
	"cl": {"chile"}, "co": {"colombia"}, "pe": {"peru"}, "uy": {"uruguay"}, "py": {"paraguay"}, "bo": {"bolivia"},
	"ec": {"ecuador"}, "ve": {"venezuela"}, "cr": {"costa", "rica"}, "pa": {"panama"}, "gt": {"guatemala"},
	"sv": {"salvador"}, "hn": {"honduras"}, "do": {"dominican", "republic"}, "jm": {"jamaica"},
	"tt": {"trinidad", "tobago"}, "pr": {"puerto", "rico"}, "ru": {"russia"}, "kz": {"kazakhstan"},
	"by": {"belarus"}, "ge": {"georgia"}, "am": {"armenia"}, "az": {"azerbaijan"}, "uz": {"uzbekistan"},
}
