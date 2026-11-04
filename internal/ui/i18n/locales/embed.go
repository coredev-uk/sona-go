// Package locales embeds the string catalogue files Sona's Go code reads:
// the English catalogue (en.po) and the reviewed mapping of Sona's messages to
// Apple Music's string keys (internal/i18n/applestrings). None holds
// Apple's text. It is a package of its own so that applestrings can
// import the files without an import cycle through internal/ui/i18n.
package locales

import _ "embed"

// SourceCatalogue is en.po: every message of Sona's interface, whose
// msgids are the source text and the identity of each message.
//
//go:embed en.po
var SourceCatalogue []byte

// AppleKeys is apple-keys.json.
//
//go:embed apple-keys.json
var AppleKeys []byte
