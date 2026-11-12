package theme

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"unicode"

	"github.com/egoist/mygo/ui"
)

// The Lucide icons (ISC, icons/LICENSE): one SVG per icon, named in
// Lucide's kebab-case. icons/README.md says which version and how to
// refresh; ALIASES.txt maps the names v0.2.0's React code used (lucide-react
// component names, older kebab names) to the canonical ones.
//
//go:embed icons/*.svg icons/ALIASES.txt
var iconFiles embed.FS

var icons struct {
	once    sync.Once
	names   map[string]bool
	aliases map[string]string

	mu     sync.Mutex
	parsed map[string]*ui.SVG
}

// fallbackIcon is drawn in production for a name that is not an icon (an
// outlined square), where tests and development builds panic.
var fallbackIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="18" height="18" x="3" y="3" rx="2"/></svg>`))

func loadIconIndex() {
	icons.names = map[string]bool{}
	icons.aliases = map[string]string{}
	icons.parsed = map[string]*ui.SVG{}
	entries, err := fs.ReadDir(iconFiles, "icons")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if n, ok := strings.CutSuffix(e.Name(), ".svg"); ok {
			icons.names[n] = true
		}
	}
	data, err := iconFiles.ReadFile("icons/ALIASES.txt")
	if err != nil {
		panic(err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && !strings.HasPrefix(f[0], "#") {
			icons.aliases[f[0]] = f[1]
		}
	}
}

// IconName resolves a name to a Lucide icon's canonical kebab-case name:
// that name itself ("chevron-right"), a name v0.2.0 used ("ChevronRight",
// "ChevronRightIcon", "loader-2", "History"). It reports false for a name
// that is no icon.
func IconName(name string) (string, bool) {
	icons.once.Do(loadIconIndex)
	if icons.names[name] {
		return name, true
	}
	for _, n := range []string{name, strings.TrimSuffix(name, "Icon"), kebab(name)} {
		if a, ok := icons.aliases[n]; ok && icons.names[a] {
			return a, true
		}
		if icons.names[n] {
			return n, true
		}
	}
	return "", false
}

// kebab is a PascalCase name in Lucide's kebab-case: ChevronRight is
// chevron-right, Maximize2 maximize-2, Disc3 disc-3.
func kebab(s string) string {
	var b strings.Builder
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsUpper(r):
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsDigit(r) && i > 0 && !unicode.IsDigit(rs[i-1]) && rs[i-1] != '-':
			b.WriteByte('-')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSuffix(b.String(), "-icon")
}

// Icon returns a Lucide icon by name, parsed once and cached: show it with
// ui.Icon(c, theme.Icon("play")), which takes the text colour and the font
// size. A name that is no icon is a bug: it panics in tests and in
// development builds and in production shows an empty square.
func Icon(name string) *ui.SVG {
	canon, ok := IconName(name)
	if !ok {
		if strict() {
			panic(fmt.Sprintf("theme: no icon %q", name))
		}
		return fallbackIcon
	}
	icons.mu.Lock()
	defer icons.mu.Unlock()
	if s := icons.parsed[canon]; s != nil {
		return s
	}
	data, err := iconFiles.ReadFile("icons/" + canon + ".svg")
	if err != nil {
		panic(err) // the name came from the embedded directory
	}
	s, err := ui.ParseSVG(data)
	if err != nil {
		panic(fmt.Sprintf("theme: icon %q: %v", canon, err))
	}
	icons.parsed[canon] = s
	return s
}
