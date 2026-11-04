package app

import (
	"sync"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// localizer shows the language of the settings: the one the user chose in
// Settings → General, else the first of the system's languages that Apple
// Music's translations cover well enough (i18n.SelectableShare); English
// when none does. Apple's text is asked of TranslationsService off the main
// thread and shown through the Poster; it also re-chooses when a locale's
// messages arrive (services.EventAppleTranslations). The account's storefront
// language (AuthStatus.Language) comes first under Automatic, as in v0.2.0,
// and the choice is made again when the account changes.
type localizer struct {
	settings *config.Store
	tr       *services.TranslationsService // nil: English, or the chosen tag without Apple's text
	post     store.Poster
	// changed runs after the language shown changed (main thread); nil:
	// nothing follows it.
	changed func()

	mu   sync.Mutex
	seq  uint64 // the latest apply, so an older answer never overwrites a newer
	last string // the Language setting the last apply saw
	done bool

	account string // the account's storefront language, "" when signed out
}

// initLocale starts following the language.
func (a *App) initLocale() {
	l := &localizer{settings: a.opts.Settings, post: a.postFunc(), changed: a.languageChanged}
	if a.opts.Core != nil {
		l.tr = services.Service[services.TranslationsService](a.opts.Core)
	}
	a.locale = l
	if a.opts.Core != nil {
		l.account = services.Service[services.AuthService](a.opts.Core).Status().Language
	}
	i18n.SetRegional(l.regional()...)
	store.On(a.opts.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
		l.mu.Lock()
		same := l.account == s.Language
		l.account = s.Language
		l.mu.Unlock()
		if !same {
			i18n.SetRegional(l.regional()...)
			l.choose(true)
		}
	})
	if l.tr != nil {
		store.On(a.opts.Bus, services.EventAppleTranslations, func(services.AppleTranslations) { l.choose(true) })
	}
	l.choose(true)
}

// regional is the system's languages, then the account's: formatting follows
// the first of them in the language shown.
func (l *localizer) regional() []string {
	tags := l10n.SystemLanguages()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.account != "" {
		tags = append(tags[:len(tags):len(tags)], l.account)
	}
	return tags
}

// preferred is the languages Automatic picks from: the account's first.
func (l *localizer) preferred() []string {
	l.mu.Lock()
	account := l.account
	l.mu.Unlock()
	sys := l10n.SystemLanguages()
	if account == "" {
		return sys
	}
	return append([]string{account}, sys...)
}

// apply re-chooses after a settings change that moved the language.
func (l *localizer) apply() { l.choose(false) }

func (l *localizer) choose(force bool) {
	lang := l.settings.Get().Language
	l.mu.Lock()
	if l.done && !force && lang == l.last {
		l.mu.Unlock()
		return
	}
	l.done, l.last = true, lang
	l.seq++
	n := l.seq
	l.mu.Unlock()

	preferred, minShare := l.preferred(), i18n.SelectableShare
	if lang != config.LanguageAutomatic {
		preferred, minShare = []string{lang}, 0
	}
	if l.tr == nil {
		if lang == config.LanguageAutomatic {
			lang = ""
		}
		i18n.SetLocale(lang)
		l.notify()
		return
	}
	go func() {
		var chosen services.AppleTranslations
		found := false
		for _, tag := range i18n.TranslationCandidates(preferred) {
			ap := l.tr.Apple(tag)
			if ap.Locale != "" && ap.Coverage > 0 && ap.Coverage >= minShare {
				chosen, found = ap, true
				break
			}
		}
		l.post(func() {
			l.mu.Lock()
			stale := n != l.seq
			l.mu.Unlock()
			if stale {
				return
			}
			if found {
				i18n.SetApple(chosen)
				i18n.SetLocale(chosen.Locale)
			} else {
				i18n.SetApple(services.AppleTranslations{})
				i18n.SetLocale("")
			}
			l.notify()
		})
	}()
}

// notify tells changed (the native menus and window titles, which no frame
// builds) that the language shown changed. Main thread.
func (l *localizer) notify() {
	if l.changed != nil {
		l.changed()
	}
}
