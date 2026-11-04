package i18n

import (
	"fmt"
	"math"
	"strings"
	"time"

	mygoi18n "github.com/egoist/mygo/i18n"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// DateStyle is the length of a date, as CLDR writes them by style.
type DateStyle uint8

const (
	// DateShort is numeric: "11/3/26".
	DateShort DateStyle = iota + 1
	// DateMedium abbreviates the month: "Nov 3, 2026".
	DateMedium
	// DateLong writes it out: "November 3, 2026".
	DateLong
	// DateFull adds the weekday: "Tuesday, November 3, 2026".
	DateFull
)

// FormatNumber writes a number in the shown language: "1,234".
func FormatNumber(n float64) string {
	return mygoi18n.FormatNumber(current.Load().format, n, mygoi18n.NumberOptions{})
}

// FormatDate writes a date in the shown language's conventions (its
// formats: an English UI on an en-GB system writes day before month).
func FormatDate(t time.Time, style DateStyle) string {
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Date: mygoiStyle(style)})
}

func mygoiStyle(s DateStyle) mygoi18n.Style {
	switch s {
	case DateShort:
		return mygoi18n.ShortStyle
	case DateMedium:
		return mygoi18n.MediumStyle
	case DateLong:
		return mygoi18n.LongStyle
	default:
		return mygoi18n.FullStyle
	}
}

// FormatClock writes the time of day, hours and minutes, in the shown
// language's hour cycle: "21:30" or "9:30 PM" (v0.2.0's clockTime).
func FormatClock(t time.Time) string {
	return mygoi18n.FormatDate(current.Load().format, t, mygoi18n.DateOptions{Skeleton: "jm"})
}

// FormatDuration writes a playback time as m:ss, or h:mm:ss from an hour
// (v0.2.0's formatTime); negative is zero.
func FormatDuration(d time.Duration) string {
	total := max(0, int64(d/time.Second))
	h, m, s := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatBytes writes a size as "12.4 MB" in the shown language's numbers
// (v0.2.0's formatBytes): whole bytes below a kilobyte, then one digit under
// ten of a unit and none from there; units are B, kB, MB, GB.
func FormatBytes(n int64) string {
	if n < 1024 {
		return FormatNumber(float64(max(n, 0))) + " B"
	}
	v, i := float64(n)/1024, 0
	units := [...]string{"kB", "MB", "GB"}
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 {
		v = math.Round(v*10) / 10
	} else {
		v = math.Round(v)
	}
	return FormatNumber(v) + " " + units[i]
}

// listWords are the words and punctuation that join a list in a language,
// from CLDR's list patterns ("A, B, and C"): the separator, the joiner of
// the last two with the separator before it when there are three or more,
// and the joiner of exactly two. Languages without an entry use English.
type listWords struct {
	sep      string // between items: ", "
	and3     string // before the last of three or more: ", and "
	and2     string // between two: " and "
	or3, or2 string // the disjunction's
}

var lists = map[string]listWords{
	"en":    {", ", ", and ", " and ", ", or ", " or "},
	"en-GB": {", ", " and ", " and ", " or ", " or "},
	"en-AU": {", ", " and ", " and ", " or ", " or "},
	"de":    {", ", " und ", " und ", " oder ", " oder "},
	"fr":    {", ", " et ", " et ", " ou ", " ou "},
	"es":    {", ", " y ", " y ", " o ", " o "},
	"it":    {", ", " e ", " e ", " o ", " o "},
	"pt":    {", ", " e ", " e ", " ou ", " ou "},
	"nl":    {", ", " en ", " en ", " of ", " of "},
	"sv":    {", ", " och ", " och ", " eller ", " eller "},
	"da":    {", ", " og ", " og ", " eller ", " eller "},
	"nb":    {", ", " og ", " og ", " eller ", " eller "},
	"fi":    {", ", " ja ", " ja ", " tai ", " tai "},
	"pl":    {", ", " i ", " i ", " lub ", " lub "},
	"cs":    {", ", " a ", " a ", " nebo ", " nebo "},
	"ru":    {", ", " и ", " и ", " или ", " или "},
	"uk":    {", ", " і ", " і ", " або ", " або "},
	"tr":    {", ", " ve ", " ve ", " veya ", " veya "},
	"ja":    {"、", "、", "、", "、", "、"},
	"zh":    {"、", "和", "和", "或", "或"},
	"ko":    {", ", " 및 ", " 및 ", " 또는 ", " 또는 "},
}

// List joins items as a sentence in the shown language, as
// Intl.ListFormat's conjunction does: "A, B, and C" in English, "A, B et C"
// in French.
func List(items []string) string { return joinList(items, false) }

// ListOr joins items with the disjunction: "A, B, or C".
func ListOr(items []string) string { return joinList(items, true) }

func joinList(items []string, or bool) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	w := wordsFor(current.Load().format)
	last3, last2 := w.and3, w.and2
	if or {
		last3, last2 = w.or3, w.or2
	}
	if len(items) == 2 {
		return items[0] + last2 + items[1]
	}
	return strings.Join(items[:len(items)-1], w.sep) + last3 + items[len(items)-1]
}

// wordsFor finds a locale's list words: its own tag, then its language.
func wordsFor(tag string) listWords {
	if w, ok := lists[tag]; ok {
		return w
	}
	if w, ok := lists[langOf(tag)]; ok {
		return w
	}
	return lists["en"]
}

// Collator orders strings by the shown language's rules
// (golang.org/x/text/collate). A collator is not safe for concurrent use:
// call it where it is needed rather than keeping it across goroutines. The
// options are collate's (collate.Numeric sorts "Track 2" before
// "Track 10").
func Collator(opts ...collate.Option) *collate.Collator {
	tag := language.Make(current.Load().format)
	if mygoi18n.IsPseudo(current.Load().locale) {
		tag = language.English
	}
	return collate.New(tag, opts...)
}
