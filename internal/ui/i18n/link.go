package i18n

import "strings"

// SplitLink splits a message with one link in its sentence, marked <0>text</0>
// (or <0>text<1/></0>, for a link with an icon), into the words before, the
// link's text and the words after. A message that lost its tags in
// translation comes back with ok false: whole as before for a message with no
// <0>, and for an unclosed <0> with what follows it as the link (after empty),
// as strings.Cut would split it.
func SplitLink(msg string) (before, link, after string, ok bool) {
	before, rest, found := strings.Cut(msg, "<0>")
	if !found {
		return msg, "", "", false
	}
	if link, after, ok = strings.Cut(rest, "<1/></0>"); ok {
		return before, link, after, true
	}
	if link, after, ok = strings.Cut(rest, "</0>"); ok {
		return before, link, after, true
	}
	return before, rest, "", false
}

// tagMarkers drops a link's markers and keeps its words.
var tagMarkers = strings.NewReplacer("<0>", "", "</0>", "", "<1/>", "")

// StripTags is msg without the markers of its link: for a place that shows
// the words as plain text (a description, a span that only carries a font
// feature).
func StripTags(msg string) string { return tagMarkers.Replace(msg) }
