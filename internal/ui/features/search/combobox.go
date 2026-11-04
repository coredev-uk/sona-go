package search

import (
	"strings"
	"unicode/utf8"
)

// moveActive is the keyboard movement through the search field's suggestion
// list (v0.2.0's combobox.ts). -1 means no option is active (the caret is
// in the field): down from there goes to the first option, up to the last,
// and moving past either end returns to the field, so the list never traps
// the keyboard. delta is 1 or -1.
func moveActive(active, delta, count int) int {
	if count <= 0 {
		return -1
	}
	if active < 0 {
		if delta > 0 {
			return 0
		}
		return count - 1
	}
	next := active + delta
	if next < 0 || next >= count {
		return -1
	}
	return next
}

// splitMatch splits text around the first case-insensitive occurrence of
// typed (outer spaces ignored), so a completion can set what the user typed
// apart from what it adds. Nothing typed or no match: the whole text is the
// remainder (after).
func splitMatch(text, typed string) (before, match, after string) {
	t := strings.TrimSpace(typed)
	if t == "" {
		return "", "", text
	}
	n := utf8.RuneCountInString(t)
	for i := range text {
		end := i
		rest := text[i:]
		count := 0
		for j := range rest {
			if count == n {
				end = i + j
				break
			}
			count++
		}
		if count < n {
			return "", "", text // fewer runes left than typed
		}
		if count == n && end == i {
			end = len(text)
		}
		if strings.EqualFold(text[i:end], t) {
			return text[:i], text[i:end], text[end:]
		}
	}
	return "", "", text
}
