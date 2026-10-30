// Package enginepage embeds the MusicKit host page run by the playback engine.
package enginepage

import _ "embed"

// URL is where the engine serves the page. The path does not exist on Apple's
// server; the engine fulfils it locally so the document's origin is
// https://music.apple.com.
const URL = "https://music.apple.com/sona-engine/"

// HTML is the engine page document.
//
//go:embed index.html
var HTML []byte
