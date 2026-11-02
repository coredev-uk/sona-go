// Package i18n embeds the string catalogue files Go reads at run time
// (internal/i18n/applestrings): Sona's English catalogue, whose messages
// Apple's translations are counted against, and the reviewed mapping of
// Sona's messages to Apple Music's string keys. Neither holds Apple's text.
package i18n

import _ "embed"

// SourceCatalogue is locales/en.po.
//
//go:embed locales/en.po
var SourceCatalogue []byte

// AppleKeys is apple-keys.json.
//
//go:embed apple-keys.json
var AppleKeys []byte
