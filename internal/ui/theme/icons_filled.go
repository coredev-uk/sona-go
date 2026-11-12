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

// solidIcons are the solid forms of a few Lucide glyphs (ISC) that are not
// the outline with its fill turned on: the play disc's triangle and the
// transport's bars drawn without their stroke, a thumb with its stroked
// stem, the sleep timer's moon (lucide-react's fill="currentColor").
var solidIcons = map[string]string{
	"play":        `<path stroke="none" d="M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z"/>`,
	"thumbs-down": `<path d="M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22a3.13 3.13 0 0 1-3-3.88Z"/><path d="M17 14V2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>`,
	"pause":       `<rect stroke="none" x="14" y="3" width="5" height="18" rx="1"/><rect stroke="none" x="5" y="3" width="5" height="18" rx="1"/>`,
	"moon":        `<path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/>`,
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
	var data []byte
	if body, ok := solidIcons[canon]; ok {
		data = []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`)
	} else {
		var err error
		if data, err = iconFiles.ReadFile("icons/" + canon + ".svg"); err != nil {
			panic(err) // the name came from the embedded directory
		}
		data = bytes.Replace(data, []byte(`fill="none"`), []byte(`fill="currentColor"`), 1)
	}
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
