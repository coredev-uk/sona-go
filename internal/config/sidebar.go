package config

import (
	"slices"
)

// Sidebar is what the sidebar remembers about the user's playlist tree
// (v0.2.0 kept it in localStorage as sona.playlist-folders.open and
// sona.sidebar.collapsed). It describes this computer's library view, so
// settings exports leave it out.
type Sidebar struct {
	// OpenFolders are the IDs of the playlist folders shown open; folders
	// start closed.
	OpenFolders []string `json:"openFolders"`
	// FoldedSections are the sidebar sections folded shut; sections start
	// open.
	FoldedSections []string `json:"foldedSections"`
}

// normalized sorts both lists, dropping empty and repeated entries, so the
// file is stable and never grows; an empty list is nil, as in the defaults.
func (s Sidebar) normalized() Sidebar {
	s.OpenFolders = tidyIDs(s.OpenFolders)
	s.FoldedSections = tidyIDs(s.FoldedSections)
	return s
}

func tidyIDs(in []string) []string {
	var out []string
	for _, id := range in {
		if id != "" {
			out = append(out, id)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
