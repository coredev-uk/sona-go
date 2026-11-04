package config

import "regexp"

// LanguageAutomatic is [Settings.Language]'s default: the interface follows
// the account's storefront language and the system's languages
// (docs/ui-architecture.md, "Strings and locales").
const LanguageAutomatic = ""

// bcp47Tag is the shape of a BCP 47 tag Sona stores: a language, then
// script, region or variant subtags ("en", "pt-BR", "zh-Hant-TW").
var bcp47Tag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// normalizedLanguage keeps a well-formed BCP 47 tag (Settings → General →
// Language) and turns anything else into Automatic. Whether Sona has a
// catalogue for the tag is decided where it is used: a tag without one
// shows English.
func normalizedLanguage(tag string) string {
	if !bcp47Tag.MatchString(tag) {
		return LanguageAutomatic
	}
	return tag
}
