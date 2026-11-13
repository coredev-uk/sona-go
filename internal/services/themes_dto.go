package services

// EventThemesChanged carries the files of the user's themes folder
// ([]ThemeFile) whenever one is added, changed or removed.
const EventThemesChanged = "themes:changed"

// MaxThemeFileBytes is the largest theme file Sona reads: a theme is a few
// kilobytes of JSON (docs/themes.md).
const MaxThemeFileBytes = 256 << 10

// maxThemeFiles is how many files of the folder are read; the rest are
// ignored.
const maxThemeFiles = 100

// ThemeFile is one *.json file of the user's themes folder, as read. Sona's
// theme registry in the UI parses and checks it (internal/ui/theme).
type ThemeFile struct {
	// Name is the file name, without its folder.
	Name string `json:"name"`
	// Data is the file's bytes; empty when TooLarge or Unreadable.
	Data []byte `json:"data"`
	// Size is the file's size in bytes.
	Size int64 `json:"size"`
	// ModTime is when the file last changed, Unix ms.
	ModTime int64 `json:"modTime"`
	// TooLarge: the file is over MaxThemeFileBytes and was not read.
	TooLarge bool `json:"tooLarge"`
	// Unreadable: the file could not be read (permissions, a vanished file).
	Unreadable bool `json:"unreadable"`
}
