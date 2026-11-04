// Package i18n words Sona's interface (docs/ui-architecture.md, "Strings
// and locales"). A string is a message written in English at its call site,
//
//	i18n.T("Add music by {name} to collect it here.", "name", artist)
//	i18n.TC("verb", "Play")              // with a msgctxt
//
// and the same text is its msgid in the catalogue, locales/en.po, which
// `go run ./cmd/i18ntool extract` keeps from the calls: msg and context
// MUST be string literals, or the tool cannot see them. Messages are ICU
// MessageFormat, formatted by MyGo ({name}, {0}, {n, plural, one {# song}
// other {# songs}}, CLDR plural rules and number formats).
//
// The text shown comes from the first of three layers that has the message:
// Apple Music's own translation for the locale (services.AppleTranslations,
// keyed by applestrings.MessageID, set with SetApple), the community
// catalogue (LoadCommunity), then English, the call site's own text.
package i18n

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// layers is what the text comes from, replaced as a whole when it changes
// (readers load the pointer, never lock).
type layers struct {
	locale  string // BCP 47 tag shown: "en", "fr-FR", "en-XA"
	locales []string
	// format is the locale numbers, dates and plural rules follow: the
	// first of locales.
	format string

	appleLocale string
	apple       map[string]string // message ID → ICU text
	community   map[string]string // message ID → ICU text, for the locale

	// resolved caches each message's parsed text for these layers.
	resolved sync.Map // msgKey → *message
}

type msgKey struct{ msg, ctx string }

// message is a resolved message: plain text, or ICU to format.
type message struct {
	plain string
	icu   *mygoi18n.Message // nil for plain text
}

var (
	current atomic.Pointer[layers]

	// settings is what builds the layers; guarded by mu.
	mu            sync.Mutex
	shown         = SourceLocale
	regional      []string // the system's and the account's tags, for formats
	appleLocale   string
	appleMessages map[string]string
	communities   = map[string]map[string]string{} // tag → message ID → ICU text
	post          store.Poster
	log           = slog.Default()
	warned        sync.Map
)

func init() { rebuild() }

// rebuild makes the layers from the settings. Call with mu held, or at
// init.
func rebuild() {
	l := &layers{locale: shown}
	l.locales = FormatLocales(shown, regional)
	l.format = l.locales[0]
	if appleLocale != "" {
		if _, ok := MatchLocale([]string{shown}, []string{appleLocale}); ok && !isSource(shown) {
			l.appleLocale, l.apple = appleLocale, appleMessages
		}
	}
	if !isSource(shown) {
		if tag, ok := MatchLocale([]string{shown}, communityTags()); ok {
			l.community = communities[tag]
		}
	}
	current.Store(l)
}

func isSource(tag string) bool {
	return langOf(strings.ToLower(tag)) == SourceLocale && tag != PseudoLocale && !strings.EqualFold(tag, PseudoLocale)
}

func communityTags() []string {
	tags := make([]string, 0, len(communities))
	for t := range communities {
		tags = append(tags, t)
	}
	return tags
}

// changed redraws the windows after the layers changed.
func changed() {
	if post != nil {
		post(func() {})
	}
}

// SetPoster says how a change of the language redraws: the app's Poster,
// which runs the func on the main thread and redraws every window.
func SetPoster(p store.Poster) {
	mu.Lock()
	post = p
	mu.Unlock()
}

// SetLogger sets where a message that does not parse is reported (once).
func SetLogger(l *slog.Logger) {
	mu.Lock()
	log = l
	mu.Unlock()
}

// Locale is the BCP 47 tag of the language shown: "en" for English.
func Locale() string { return current.Load().locale }

// Locales are the tags numbers and dates are written for, best first
// (FormatLocales): the first is what the formatters use.
func Locales() []string { return current.Load().locales }

// SetLocale shows a language: its tag, "" or "en" for English, "en-XA" for
// the pseudo-locale. The layers set for it (SetApple, LoadCommunity) apply;
// the windows redraw through the Poster.
func SetLocale(tag string) {
	if tag == "" {
		tag = SourceLocale
	}
	mu.Lock()
	shown = tag
	rebuild()
	mu.Unlock()
	changed()
}

// SetRegional sets the system's and the account's language tags, in order
// of preference: numbers and dates follow the ones of the shown language
// (an English UI on an en-GB system keeps 24-hour clocks).
func SetRegional(tags ...string) {
	mu.Lock()
	regional = append([]string(nil), tags...)
	rebuild()
	mu.Unlock()
	changed()
}

// SetApple sets Apple Music's translations of Sona's messages for its
// locale (TranslationsService.Apple, and the "translations:apple" event).
// They show while that locale (or a sibling, "fr-CA" for "fr-FR") is the
// one shown. A message that does not parse is left out and shown in the
// next layer.
func SetApple(a services.AppleTranslations) {
	mu.Lock()
	appleLocale, appleMessages = a.Locale, a.Messages
	rebuild()
	mu.Unlock()
	changed()
}

// LoadCommunity adds a community catalogue (a gettext .po file keyed by the
// English msgids) for a locale. Messages it translates show when the
// locale (or a sibling) is shown and Apple's layer lacks them.
func LoadCommunity(tag string, data []byte) error {
	f, err := po.Parse(data)
	if err != nil {
		return fmt.Errorf("i18n: community catalogue %s: %w", tag, err)
	}
	msgs := make(map[string]string, len(f.Entries))
	for _, e := range f.Entries {
		if !e.Obsolete && !e.Fuzzy() && e.Str != "" {
			msgs[applestrings.MessageID(e.ID, e.Context)] = e.Str
		}
	}
	mu.Lock()
	communities[tag] = msgs
	rebuild()
	mu.Unlock()
	changed()
	return nil
}

// T is msg in the language shown. kv are the message's arguments as name,
// value pairs: T("Add music by {name} to collect it here.", "name", artist).
// Lingui's positional placeholders ({0}, {1}) are named "0" and "1" (or the
// integers 0 and 1). msg MUST be a string literal.
func T(msg string, kv ...any) string { return TC("", msg, kv...) }

// TC is T for a message with a context (msgctxt): the same English word
// can be worded differently by what it means. context and msg MUST be
// string literals.
func TC(context, msg string, kv ...any) string {
	l := current.Load()
	m := l.resolve(context, msg)
	if m.icu == nil {
		return m.plain
	}
	return m.icu.Format(l.format, args(msg, kv))
}

func (l *layers) resolve(ctx, msg string) *message {
	k := msgKey{msg, ctx}
	if v, ok := l.resolved.Load(k); ok {
		return v.(*message)
	}
	m := l.build(ctx, msg)
	v, _ := l.resolved.LoadOrStore(k, m)
	return v.(*message)
}

// build picks the layer's text for a message and parses it. A translation
// that does not parse is left out and logged once: Apple's and the
// community's text is not ours to fix.
func (l *layers) build(ctx, msg string) *message {
	pseudo := mygoi18n.IsPseudo(l.locale)
	if l.apple != nil || l.community != nil {
		id := applestrings.MessageID(msg, ctx)
		for _, layer := range []map[string]string{l.apple, l.community} {
			text, ok := layer[id]
			if !ok || text == "" {
				continue
			}
			m, err := parse(text, pseudo)
			if err != nil {
				logBroken(text, err)
				continue
			}
			return m
		}
	}
	m, err := parse(msg, pseudo)
	if err != nil {
		// The call site's own text is a bug: tests and dev builds stop.
		logBroken(msg, err)
		if strict() {
			panic(fmt.Sprintf("i18n: message %q does not parse: %v", msg, err))
		}
		return &message{plain: msg}
	}
	return m
}

// parse reads an ICU message; text with no syntax stays plain (except
// under a pseudo-locale, which transforms every message).
func parse(text string, pseudo bool) (*message, error) {
	if !pseudo && !strings.ContainsAny(text, "{}'") {
		return &message{plain: text}, nil
	}
	icu, err := mygoi18n.ParseMessage(text)
	if err != nil {
		return nil, err
	}
	return &message{icu: icu}, nil
}

func logBroken(text string, err error) {
	if _, seen := warned.LoadOrStore(text, true); seen {
		return
	}
	mu.Lock()
	lg := log
	mu.Unlock()
	lg.Warn("message does not parse", "message", text, "err", err)
}

// strict says a call's mistakes are bugs: tests panic on them.
func strict() bool { return testing.Testing() }

// args turns T's name, value pairs into the formatter's arguments.
func args(msg string, kv []any) map[string]any {
	if len(kv) == 0 {
		return nil
	}
	if len(kv)%2 != 0 && strict() {
		panic(fmt.Sprintf("i18n: %q: arguments are name, value pairs", msg))
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		var name string
		switch k := kv[i].(type) {
		case string:
			name = k
		case int:
			name = strconv.Itoa(k)
		default:
			if strict() {
				panic(fmt.Sprintf("i18n: %q: argument name %v (%T) is not a string or an int", msg, k, k))
			}
			name = fmt.Sprint(k)
		}
		m[name] = kv[i+1]
	}
	return m
}
