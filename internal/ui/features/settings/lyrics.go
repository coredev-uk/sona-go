package settings

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// translationLanguages are the languages offered for translations ("" is
// the Apple Music account's).
var translationLanguages = []string{
	"ar", "de", "en", "es", "fr", "he", "hi", "id", "it", "ja", "ko", "nl", "pl", "pt", "pt-BR", "ru", "sv", "th",
	"tr", "uk", "vi", "zh-Hans", "zh-Hant",
}

const accountLanguage = "account"

// moveProvider is the list with the provider at from moved one place up
// (-1) or down (1); unchanged at an end (lyrics-order.ts).
func moveProvider(list []config.LyricsProvider, from, dir int) []config.LyricsProvider {
	next := slices.Clone(list)
	to := from + dir
	if to < 0 || to >= len(list) {
		return next
	}
	next[from], next[to] = next[to], next[from]
	return next
}

// languageName is the language's name in the interface's language; the tag
// when it can't be named.
func languageName(tag string) string {
	t, err := language.Parse(tag)
	if err != nil {
		return tag
	}
	if n := display.Languages(language.Make(i18n.Locale())).Name(t); n != "" {
		return n
	}
	return tag
}

// translationItems are the select's items: the account's language, then
// the languages named in the UI locale and collator-sorted; a saved tag
// outside the list is appended.
func translationItems(saved string) []kit.SelectItem {
	tags := slices.Clone(translationLanguages)
	if saved != "" && !slices.Contains(tags, saved) {
		tags = append(tags, saved)
	}
	type named struct{ tag, name string }
	list := make([]named, len(tags))
	for i, t := range tags {
		list[i] = named{t, languageName(t)}
	}
	col := i18n.Collator()
	slices.SortStableFunc(list, func(a, b named) int { return col.CompareString(a.name, b.name) })
	items := []kit.SelectItem{kit.SelectOption(accountLanguage, i18n.T("Your Apple Music language"))}
	for _, l := range list {
		items = append(items, kit.SelectOption(l.tag, l.name))
	}
	return items
}

type lyricsState struct{ focus string }

type lyricsAbout struct{ name, about string }

func lyricsSource(id string) lyricsAbout {
	switch id {
	case config.LyricsApple:
		return lyricsAbout{i18n.T("Apple Music"), i18n.T("Word-by-word and line-by-line lyrics, with translations and pronunciation.")}
	case config.LyricsMusixmatch:
		return lyricsAbout{i18n.T("Musixmatch"), i18n.T("Word-by-word and line-by-line lyrics, with translations from its community.")}
	case config.LyricsLRCLIB:
		return lyricsAbout{i18n.T("LRCLIB"), i18n.T("Line-by-line lyrics from an open, shared database.")}
	case config.LyricsNetEase:
		return lyricsAbout{i18n.T("NetEase Cloud Music"), i18n.T("Line-by-line lyrics, strongest for Chinese, Japanese and Korean music.")}
	}
	return lyricsAbout{name: id}
}

// lyricsGroup is Settings → Playback → Lyrics: the sources in order with
// their switches, word-by-word timing, and the translation language.
func (s *settings) lyricsGroup(c *ui.Context, st config.Settings) {
	ly := st.Lyrics
	s.group(c, "playback-lyrics", i18n.T("Lyrics"),
		i18n.T("Sona asks every source that's on and shows the lyrics with the finest timing: word by word, then line by line, then plain text. A source higher in the list wins a tie."),
		func() {
			s.lyricsSources(c, ly)
			s.switchRow(c, rowOpts{
				ID:    "lyrics-word-timing",
				Title: i18n.T("Prefer word-by-word lyrics"),
				Desc:  i18n.T("Lyrics timed word by word win over line-by-line lyrics from a source higher in the list."),
			}, ly.PreferWordTiming, false, func(on bool) {
				s.save(func(x *config.Settings) { x.Lyrics.PreferWordTiming = on })
			})
			value := ly.TranslationLanguage
			if value == "" {
				value = accountLanguage
			}
			s.selectRow(c, rowOpts{
				ID:    "lyrics-translation",
				Title: i18n.T("Lyrics translation"),
				Desc:  i18n.T("Shown under each line where a source has a translation into this language."),
			}, value, translationItems(ly.TranslationLanguage), false, func(v string) {
				if v == accountLanguage {
					v = ""
				}
				s.save(func(x *config.Settings) { x.Lyrics.TranslationLanguage = v })
			})
		})
}

// lyricsSources is the list labelled Lyrics sources (the find target).
func (s *settings) lyricsSources(c *ui.Context, ly config.Lyrics) {
	const id = "lyrics-sources"
	st := stateOf[lyricsState](s, "lyrics")
	c = c.Key("row:" + id)
	s.seen[id] = true
	n := len(ly.Providers)
	var first ui.Element
	list := ui.Column(c).FillWidth().Role(ui.RoleList).Label(i18n.T("Lyrics sources"))
	s.wash(list, id)
	list.Children(func() {
		for i, p := range ly.Providers {
			k := c.Key(p.ID)
			about := lyricsSource(p.ID)
			ui.Column(k).FillWidth().Role(ui.RoleListItem).Children(func() {
				s.line(k)
				item := ui.Row(k.Key("item")).FillWidth().Padding(rowPadY, 0).AlignItems(ui.Center).Gap(24)
				item.Children(func() {
					ui.Column(k.Key("text")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(rowTextWidth).Children(func() {
						text(k, about.name, "label").TextColor(theme.C("ink"))
						if about.about != "" {
							text(k, about.about, "secondary").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
						}
					})
					ui.Row(k.Key("controls")).Shrink(0).AlignItems(ui.Center).Gap(4).Children(func() {
						for _, dir := range []int{-1, 1} {
							icon, label, off := "chevron-up", i18n.T("Move {name} up", "name", about.name), i == 0
							if dir == 1 {
								icon, label, off = "chevron-down", i18n.T("Move {name} down", "name", about.name), i == n-1
							}
							key := p.ID + ":" + strconv.Itoa(dir)
							b := kit.IconButton(k.Key(key), icon, label, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: off})
							if st.focus == key && !off {
								st.focus = ""
								b.Focus()
							}
							if b.Clicked() {
								s.moveLyrics(st, p.ID, dir)
							}
						}
						on := p.Enabled
						sw := kit.Switch(k.Key("on"), &on, kit.SwitchOpts{A11yLabel: about.name})
						sw.Margin(0, 0, 0, 12).Description(about.about)
						if first == (ui.Element{}) {
							first = sw
						}
						if sw.Changed() {
							s.save(func(x *config.Settings) {
								for j := range x.Lyrics.Providers {
									if x.Lyrics.Providers[j].ID == p.ID {
										x.Lyrics.Providers[j].Enabled = on
									}
								}
							})
						}
					})
				})
			})
		}
	})
	s.land(c, id, list, first)
}

// moveLyrics moves a source one place and keeps the focus on the button
// that moved it, or on its other button once the row reaches an end.
func (s *settings) moveLyrics(st *lyricsState, id string, dir int) {
	s.save(func(x *config.Settings) {
		i := slices.IndexFunc(x.Lyrics.Providers, func(p config.LyricsProvider) bool { return p.ID == id })
		if i < 0 {
			return
		}
		to := i + dir
		if to < 0 || to >= len(x.Lyrics.Providers) {
			return
		}
		x.Lyrics.Providers = moveProvider(x.Lyrics.Providers, i, dir)
		d := dir
		if to == 0 || to == len(x.Lyrics.Providers)-1 {
			d = -dir
		}
		st.focus = id + ":" + strconv.Itoa(d)
	})
}
