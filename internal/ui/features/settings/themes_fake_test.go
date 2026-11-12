package settings

import "sync"

// fakeThemes stands in for the themes service: a fixed folder, and a record
// of the folder being opened.
type fakeThemes struct {
	mu     sync.Mutex
	opened int
	err    error
}

const fakeThemesFolder = "/home/you/.config/sona/themes"

func (f *fakeThemes) Folder() string { return fakeThemesFolder }

func (f *fakeThemes) OpenFolder() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened++
	return f.err
}

func (f *fakeThemes) opens() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opened
}

// withFakeThemes installs the fake before the first frame (a fixed folder
// keeps the pictures the same on every machine).
func withFakeThemes(s *settings) *fakeThemes {
	f := &fakeThemes{}
	stateOf[themeSeamState](s, "seam:themes").api = f
	return f
}
