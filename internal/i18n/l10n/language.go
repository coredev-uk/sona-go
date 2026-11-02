package l10n

import (
	"maps"
	"slices"
	"strings"
	"sync"
)

// Language is the user's language for Go's own text and for Apple's
// content: the Language setting (Settings → General) when one is chosen,
// else the system's languages. It is safe for concurrent use.
type Language struct {
	mu      sync.Mutex
	setting string
	system  []string
	printer *Printer
	subs    map[int]func()
	nextSub int
}

// NewLanguage starts from the setting ("" for Automatic) and the system
// languages the environment names (EnvLanguages); DetectSystem refines
// those with the system's own list.
func NewLanguage(setting string) *Language {
	l := &Language{setting: setting, system: EnvLanguages(), subs: map[int]func(){}}
	l.printer = For(l.preferredLocked())
	return l
}

func (l *Language) preferredLocked() []string {
	if l.setting != "" {
		return []string{l.setting}
	}
	return slices.Clone(l.system)
}

// Preferred lists the BCP 47 tags to show, in order: the setting alone, or
// the system's languages under Automatic. Empty means English.
func (l *Language) Preferred() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.preferredLocked()
}

// Printer shows Go's messages in the preferred language.
func (l *Language) Printer() *Printer {
	if l == nil {
		return English
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.printer
}

// SetSetting applies the Language setting ("" for Automatic).
func (l *Language) SetSetting(tag string) {
	l.update(func() { l.setting = tag })
}

// SetSystem replaces the system's languages (DetectSystem).
func (l *Language) SetSystem(tags []string) {
	l.update(func() { l.system = slices.Clone(tags) })
}

// DetectSystem reads the system's languages (SystemLanguages) in the
// background, so a slow answer never holds the first paint, and applies
// them.
func (l *Language) DetectSystem() {
	go func() { l.SetSystem(SystemLanguages()) }()
}

// update applies fn and, when the preferred languages changed, tells the
// subscribers (outside the lock).
func (l *Language) update(fn func()) {
	l.mu.Lock()
	before := l.preferredLocked()
	fn()
	after := l.preferredLocked()
	if slices.Equal(before, after) {
		l.mu.Unlock()
		return
	}
	l.printer = For(after)
	subs := make([]func(), 0, len(l.subs))
	for _, id := range slices.Sorted(maps.Keys(l.subs)) {
		subs = append(subs, l.subs[id])
	}
	l.mu.Unlock()
	for _, fn := range subs {
		fn()
	}
}

// OnChange calls fn after the preferred languages change (the setting, or
// the system's list once detected). The returned function unsubscribes.
func (l *Language) OnChange(fn func()) func() {
	l.mu.Lock()
	defer l.mu.Unlock()
	id := l.nextSub
	l.nextSub++
	l.subs[id] = fn
	return func() {
		l.mu.Lock()
		delete(l.subs, id)
		l.mu.Unlock()
	}
}

// posixTag turns a POSIX locale name ("fr_CA.UTF-8@euro") into a BCP 47 tag
// ("fr-CA"); "" for the C and POSIX locales, which mean untranslated.
func posixTag(v string) string {
	v, _, _ = strings.Cut(v, ".")
	v, _, _ = strings.Cut(v, "@")
	v = strings.TrimSpace(v)
	if v == "" || v == "C" || v == "POSIX" {
		return ""
	}
	return strings.ReplaceAll(v, "_", "-")
}

// envLanguages reads the gettext environment: LANGUAGE's list (honoured
// unless the locale is C or POSIX), then the locale of LC_ALL, LC_MESSAGES
// or LANG, the first one set.
func envLanguages(getenv func(string) string) []string {
	locale := ""
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(k); v != "" {
			locale = v
			break
		}
	}
	main := posixTag(locale)
	if main == "" {
		return nil
	}
	var out []string
	for _, v := range strings.Split(getenv("LANGUAGE"), ":") {
		if t := posixTag(v); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if !slices.Contains(out, main) {
		out = append(out, main)
	}
	return out
}
