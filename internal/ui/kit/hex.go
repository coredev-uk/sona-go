package kit

import "github.com/egoist/mygo/ui"

// ParseHex parses a "#rrggbb" colour as the services send them; false for ""
// or anything else (ui.Hex would panic).
func ParseHex(s string) (ui.Color, bool) {
	if len(s) != 7 || s[0] != '#' {
		return ui.Color{}, false
	}
	for i := 1; i < 7; i++ {
		if ch := s[i]; (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return ui.Color{}, false
		}
	}
	return ui.Hex(s), true
}
