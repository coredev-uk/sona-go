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

// SidebarAppleSection is the ID of the sidebar's Apple Music section in
// FoldedSections.
const SidebarAppleSection = "apple-music"

// legacyMadeForYou is the ID of the Made for you section, which was merged
// into Apple Music.
const legacyMadeForYou = "made-for-you"

// normalized sorts both lists, dropping empty and repeated entries, so the
// file is stable and never grows; an empty list is nil, as in the defaults.
// A section folded under the old Made for you ID is read as the merged Apple
// Music section folded, and the old ID is dropped, so the next save writes
// only the new one.
func (s Sidebar) normalized() Sidebar {
	s.OpenFolders = tidyIDs(s.OpenFolders)
	if slices.Contains(s.FoldedSections, legacyMadeForYou) {
		s.FoldedSections = append(slices.DeleteFunc(slices.Clone(s.FoldedSections), func(id string) bool { return id == legacyMadeForYou }), SidebarAppleSection)
	}
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
