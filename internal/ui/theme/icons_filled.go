package theme

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/egoist/mygo/ui"
)

var filledIcons struct {
	mu     sync.Mutex
	parsed map[string]*ui.SVG
}

// IconFilled is Icon drawn filled (lucide-react's `fill="currentColor"`): a
// loved heart, a favourited star, a set thumb. Cached like Icon; an unknown
// name behaves as Icon's.
func IconFilled(name string) *ui.SVG {
	canon, ok := IconName(name)
	if !ok {
		return Icon(name)
	}
	filledIcons.mu.Lock()
	defer filledIcons.mu.Unlock()
	if s := filledIcons.parsed[canon]; s != nil {
		return s
	}
	data, err := iconFiles.ReadFile("icons/" + canon + ".svg")
	if err != nil {
		panic(err) // the name came from the embedded directory
	}
	data = bytes.Replace(data, []byte(`fill="none"`), []byte(`fill="currentColor"`), 1)
	s, err := ui.ParseSVG(data)
	if err != nil {
		panic(fmt.Sprintf("theme: icon %q: %v", canon, err))
	}
	if filledIcons.parsed == nil {
		filledIcons.parsed = map[string]*ui.SVG{}
	}
	filledIcons.parsed[canon] = s
	return s
}
