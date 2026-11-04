package config

// LyricsView is what the lyrics view remembers (v0.2.0 kept it in
// localStorage under sona.lyrics): which lines it adds under the sung words,
// for every song. It describes how this person reads lyrics here, so settings
// exports leave it out.
type LyricsView struct {
	// Translation adds the line translated into the chosen language.
	Translation bool `json:"translation"`
	// Pronunciation adds the words in Latin script.
	Pronunciation bool `json:"pronunciation"`
}
