package theme

import (
	"slices"
	"strings"
	"time"
)

// DefaultThemeID is the theme used until another is chosen: Sona's own.
const DefaultThemeID = "sona"

// UserFile is one file of the user's themes folder as the themes service
// listed it (services.ThemeFile).
type UserFile struct {
	// Name is the file name, without its folder.
	Name string
	// Data is the file's bytes; empty when TooLarge.
	Data []byte
	// TooLarge: the file is over the size cap and was not read.
	TooLarge bool
	// Unreadable: the file could not be read.
	Unreadable bool
	ModTime    time.Time
}

// UserEntry is what became of one user theme file.
type UserEntry struct {
	// File is the file name.
	File string
	// Theme is the theme in use from this file: the file's own, or when the
	// file is refused, the last good version of it (Kept). Nil for a file
	// that has never been good.
	Theme *Theme
	// Problems are why the file as it is now was refused; none when it is in
	// use as written.
	Problems []Problem
	// Kept: the file was refused, and Theme is an earlier version of it that
	// stays in use.
	Kept bool
}

// builtinThemes are the embedded themes, in gallery order.
var builtinThemes []*Theme

// Themes lists every theme that can be chosen: the built-ins in their order,
// then the user's by name. The list and the themes are shared and never
// changed: a new list replaces it when the user's files change.
func Themes() []*Theme {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.all
}

// Lookup finds a theme by ID.
func Lookup(id string) *Theme {
	state.mu.Lock()
	defer state.mu.Unlock()
	return lookupLocked(id)
}

func lookupLocked(id string) *Theme {
	for _, t := range state.all {
		if t.ID == id {
			return t
		}
	}
	return nil
}

// SetTheme chooses the theme (config.Settings.ThemeID). An ID that no theme
// has, as when its file was deleted, shows Sona's own theme and returns
// false; the ID is kept, so the theme is back when its file is. A theme that
// gives one variant only pins the appearance to it (Dark).
func SetTheme(id string) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if id == "" {
		id = DefaultThemeID
	}
	if state.wantID == id && state.active != nil {
		return state.active.ID == id
	}
	state.wantID = id
	resolveLocked()
	return state.active.ID == id
}

// Requested is the ID the settings ask for; Active().ID differs from it when
// no theme has that ID.
func Requested() string {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.wantID
}

// Active is the theme in use.
func Active() *Theme {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.active
}

// Revision counts the changes of the tokens in use (a theme chosen, the
// user's files reloaded, the token document replaced): what colours worked
// out from the tokens, such as the album page's, are keyed on.
func Revision() uint64 {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.rev
}

// UserEntries are the user's theme files as the last LoadUser read them, by
// file name.
func UserEntries() []UserEntry {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.entries
}

// LoadUser parses the user's theme files and makes the good ones choosable,
// replacing the earlier list. A refused file keeps the last good version of
// itself in use (UserEntry.Kept) and says why it was refused; a theme whose
// ID another already has is refused too (the built-ins first, then the files
// in name order). The chosen theme is applied again, so an edit of its file
// shows at once. Call it from the main thread, as the files change.
func LoadUser(files []UserFile) []UserEntry {
	files = slices.Clone(files)
	slices.SortFunc(files, func(a, b UserFile) int { return strings.Compare(a.Name, b.Name) })
	state.mu.Lock()
	defer state.mu.Unlock()

	taken := map[string]bool{}
	for _, t := range builtinThemes {
		taken[t.ID] = true
	}
	good := map[string]*Theme{}
	entries := make([]UserEntry, 0, len(files))
	var user []*Theme
	for _, f := range files {
		var th *Theme
		var problems []Problem
		switch {
		case f.TooLarge:
			problems = []Problem{{Kind: ProblemTooLarge}}
		case f.Unreadable:
			problems = []Problem{{Kind: ProblemUnreadable}}
		default:
			th, problems = parseTheme(f.Data, state.base)
			if th != nil && taken[th.ID] {
				th, problems = nil, []Problem{{Kind: ProblemIDTaken}}
			}
		}
		e := UserEntry{File: f.Name, Theme: th, Problems: problems}
		if th == nil {
			if kept := state.lastGood[f.Name]; kept != nil && !taken[kept.ID] {
				e.Theme, e.Kept = kept, true
			}
		}
		if e.Theme != nil {
			e.Theme.File = f.Name
			taken[e.Theme.ID] = true
			good[f.Name] = e.Theme
			user = append(user, e.Theme)
		}
		entries = append(entries, e)
	}
	slices.SortStableFunc(user, func(a, b *Theme) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	state.lastGood = good
	state.user = user
	state.entries = entries
	resolveLocked()
	return entries
}

// resolveLocked makes the requested theme the active one and drops what was
// worked out from the tokens of the last.
func resolveLocked() {
	state.all = append(slices.Clone(builtinThemes), state.user...)
	th := lookupLocked(state.wantID)
	if th == nil {
		th = builtinThemes[0]
	}
	d, err := th.document(state.base)
	if err != nil {
		// A base reloaded in development that this theme no longer fits.
		th, d = builtinThemes[0], state.base
	}
	state.active, state.doc = th, d
	state.tokens = [2]*Tokens{}
	state.themes = nil
	state.rev++
}
