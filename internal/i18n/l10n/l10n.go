// Package l10n is Go's own message catalogue: the text Sona's Go code shows
// itself, outside the UI (the tray menu, desktop notifications, the
// command line, file chooser titles and filters). The UI's text is
// internal/ui/i18n's; text Go sends to the UI is a code the UI words
// (internal/services/errors.go), never English.
//
// Messages are English source text typed [Message]: a typed constant or
// variable (`const msgQuit l10n.Message = "Quit Sona"`), a conversion
// (`l10n.Message("Quit Sona")`) or the values of a map or slice of
// Message. The marked value is a string literal (or literals joined with
// +) or the name of another message; anything else fails extraction. The
// declaration's comment is the translators' note. `go run
// ./cmd/i18ntool extract` (package extract) collects them into
// locales/en.po, and `extract -check` (also a test) fails when that file
// is out of date. A translation is locales/<tag>.po (the same msgids,
// msgstr filled in, fuzzy ones skipped), embedded in the binary. Messages
// are fmt formats: "%s" and "%d" verbs, which a translation may reorder
// ("%[2]s"); a translation whose verbs do not match the English is not
// used.
package l10n

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// Message is English source text in Go's catalogue (see the package doc
// for how to mark one so extraction finds it).
type Message string

// SourceLocale is the catalogue's language: messages are its text.
const SourceLocale = "en"

// SourceFile is the English catalogue extraction writes, relative to this
// package's directory.
const SourceFile = "locales/en.po"

//go:embed locales/*.po
var embedded embed.FS

// catalogues holds locales/*.po: the embedded files (tests swap in their
// own, then reset the cache below).
var catalogues fs.FS = embedded

// Printer shows messages in one language. The nil Printer is English.
type Printer struct {
	tag  string
	msgs map[Message]string
}

// English shows the source text.
var English = &Printer{tag: SourceLocale}

// NewPrinter returns a Printer for tag with the given translations
// (English message → text), standing in for a catalogue in other
// packages' tests.
func NewPrinter(tag string, translations map[Message]string) *Printer {
	return &Printer{tag: tag, msgs: translations}
}

// Tag is the catalogue's BCP 47 tag ("en" for the source).
func (p *Printer) Tag() string {
	if p == nil {
		return SourceLocale
	}
	return p.tag
}

// T returns m's translation, or m when the catalogue has none.
func (p *Printer) T(m Message) string {
	if p != nil {
		if s, ok := p.msgs[m]; ok {
			return s
		}
	}
	return string(m)
}

// Sprintf formats m's translation with a.
func (p *Printer) Sprintf(m Message, a ...any) string {
	return fmt.Sprintf(p.T(m), a...)
}

// verbRe matches an fmt verb with its flags, index, width and precision.
var verbRe = regexp.MustCompile(`%(?:\[\d+\])?[-+# 0]*(?:\[\d+\])?(?:\d+|\*)?(?:\.(?:\[\d+\])?(?:\d+|\*)?)?[a-zA-Z%]`)

// Verbs lists the verbs of format s without flags or indexes, sorted ("%%"
// is left out): a translation must use the same ones as its source.
func Verbs(s string) []string {
	var out []string
	for _, v := range verbRe.FindAllString(s, -1) {
		if c := v[len(v)-1:]; c != "%" {
			out = append(out, c)
		}
	}
	slices.Sort(out)
	return out
}

var (
	catMu  sync.Mutex
	cats   map[string]*Printer // lower-case tag → loaded catalogue
	tags   []string            // the translated catalogues' tags
	source map[Message]bool
)

// loadIndexLocked lists the embedded catalogues and reads the English one.
func loadIndexLocked() {
	if cats != nil {
		return
	}
	cats = map[string]*Printer{}
	source = map[Message]bool{}
	if f, err := readPO(SourceFile); err == nil {
		for _, e := range f.Entries {
			if !e.Obsolete && !e.HasContext {
				source[Message(e.ID)] = true
			}
		}
	}
	entries, _ := fs.ReadDir(catalogues, "locales")
	for _, e := range entries {
		name := e.Name()
		if tag, ok := strings.CutSuffix(name, ".po"); ok && "locales/"+name != SourceFile {
			tags = append(tags, tag)
		}
	}
}

func readPO(name string) (*po.File, error) {
	b, err := fs.ReadFile(catalogues, name)
	if err != nil {
		return nil, err
	}
	return po.Parse(b)
}

// Locales lists the tags of the translated catalogues built in (English,
// the source, is not one of them).
func Locales() []string {
	catMu.Lock()
	defer catMu.Unlock()
	loadIndexLocked()
	return slices.Clone(tags)
}

// load reads tag's catalogue once: the translations of current English
// messages, without fuzzy ones or ones whose verbs differ from the source.
func load(tag string) *Printer {
	key := strings.ToLower(tag)
	if p, ok := cats[key]; ok {
		return p
	}
	p := &Printer{tag: tag, msgs: map[Message]string{}}
	if f, err := readPO(path.Join("locales", tag+".po")); err == nil {
		for _, e := range f.Entries {
			m := Message(e.ID)
			if e.Obsolete || e.HasContext || e.Str == "" || e.Fuzzy() || !source[m] {
				continue
			}
			if !slices.Equal(Verbs(e.ID), Verbs(e.Str)) {
				continue
			}
			p.msgs[m] = e.Str
		}
	}
	cats[key] = p
	return p
}

func language(tag string) string {
	l, _, _ := strings.Cut(strings.ToLower(tag), "-")
	return l
}

// MatchLocale picks the catalogue for the first preferred tag that has one,
// as the UI does (MatchLocale in internal/ui/i18n/locale.go):
// the tag itself, its bare language, or another region of its language.
// An English tag ends the search: English answers it. "" when none.
func MatchLocale(preferred, available []string) string {
	for _, want := range preferred {
		want = strings.ToLower(strings.TrimSpace(want))
		if want == "" {
			continue
		}
		if language(want) == SourceLocale {
			return ""
		}
		i := slices.IndexFunc(available, func(a string) bool { return strings.ToLower(a) == want })
		if i < 0 {
			i = slices.IndexFunc(available, func(a string) bool { return strings.ToLower(a) == language(want) })
		}
		if i < 0 {
			i = slices.IndexFunc(available, func(a string) bool { return language(a) == language(want) })
		}
		if i >= 0 {
			return available[i]
		}
	}
	return ""
}

// For returns the Printer for the first preferred tag with a catalogue,
// else English.
func For(preferred []string) *Printer {
	catMu.Lock()
	defer catMu.Unlock()
	loadIndexLocked()
	tag := MatchLocale(preferred, tags)
	if tag == "" {
		return English
	}
	return load(tag)
}
