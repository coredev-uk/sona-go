package settings

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// automaticLanguage is the select's value for Automatic.
const automaticLanguage = "automatic"

// languagesAPI is what the Language row asks of TranslationsService.
type languagesAPI interface {
	Languages(minShare float64) services.AppleLanguages
}

// languagesSeam is a test's replacement of TranslationsService.
type languagesSeam struct{ api languagesAPI }

func (s *settings) translations() languagesAPI {
	if v := stateOf[languagesSeam](s, "seam:translations").api; v != nil {
		return v
	}
	if s.env.Core == nil {
		return nil
	}
	return services.Service[services.TranslationsService](s.env.Core)
}

// languageState is what outlives a frame: the language saved and awaiting
// its confirmation, and whether the confirmation is followed.
type languageState struct {
	following bool
	awaiting  string
	waiting   bool
}

// languageOptions are the languages the setting offers for the chosen one,
// once Go has answered. Native Sona has no community catalogues to add, so
// Apple's locales are always listed when they could be complete enough.
func (s *settings) languageOptions(current string) ([]i18n.LanguageOption, bool) {
	tr := s.translations()
	if tr == nil {
		return nil, false
	}
	need := i18n.SelectableShare
	r := store.Use(s.env.Queries, store.Key{"translations", "languages", need}, func(context.Context) (services.AppleLanguages, error) {
		return tr.Languages(need), nil
	})
	if !r.HasData {
		return nil, false
	}
	return i18n.LanguageOptions(r.Data, nil, current, i18n.Locale()), true
}

// languageRow reports whether Settings → General offers its Language row:
// once a language besides English is selectable, or while one is chosen.
func (s *settings) languageRow() bool {
	current := s.cfg().Data.Language
	opts, ok := s.languageOptions(current)
	return ok && i18n.OffersLanguages(opts, current)
}

// languagePanel is Sona's language: General's first row.
func (s *settings) languagePanel(c *ui.Context, cfg config.Settings) {
	opts, ok := s.languageOptions(cfg.Language)
	if !ok || !i18n.OffersLanguages(opts, cfg.Language) {
		return
	}
	st := stateOf[languageState](s, "language")
	if !st.following {
		st.following = true
		// Apple's content follows the language (l=): once Go has saved it,
		// every query refetches in the new one.
		store.On(s.env.Bus, services.EventSettingsChanged, func(v config.Settings) {
			if st.waiting && v.Language == st.awaiting {
				st.waiting = false
				s.env.Queries.Invalidate()
			}
		})
	}
	items := []kit.SelectItem{kit.SelectOption(automaticLanguage, i18n.T("Automatic"))}
	for _, o := range opts {
		items = append(items, kit.SelectOptionLang(o.Tag, o.Name, o.Tag))
	}
	value := cfg.Language
	if value == "" {
		value = automaticLanguage
	}
	s.bare(c.Key("language"), func() {
		s.selectRow(c, rowOpts{
			ID: "language", Title: i18n.T("Language"),
			Desc: i18n.T("Automatic follows your Apple Music account and your system. Apple Music's own text, such as album notes, follows too where your country or region offers the language."),
		}, value, items, false, func(v string) {
			lang := v
			if v == automaticLanguage {
				lang = ""
			}
			if lang == cfg.Language {
				return
			}
			st.awaiting, st.waiting = lang, true
			s.save(func(x *config.Settings) { x.Language = lang })
		})
	})
}
