package services

// EventArtistReleasesChanged carries the tracker's NewReleases whenever a
// check finds a release or the user reads or dismisses one, and after
// sign-out empties the list.
const EventArtistReleasesChanged = "artisttracker:changed"

// OpenNewReleases is the OpenTarget.Kind of Sona's own New releases page
// (no ID): what the click on a summary notification opens.
const OpenNewReleases = "new-releases"

// Release kinds of a NewRelease.
const (
	ReleaseAlbum  = "album"
	ReleaseEP     = "ep"
	ReleaseSingle = "single"
)

// NewRelease is a release of a favourite artist that the tracker found and
// the user hasn't dismissed.
type NewRelease struct {
	// Album is the catalogue album, as the item menus and the album page
	// take it; Album.Prerelease says it is not out yet (Remind me applies).
	Album Album `json:"album"`
	// Kind is ReleaseAlbum, ReleaseEP or ReleaseSingle.
	Kind string `json:"kind"`
	// FoundAt is when the tracker found it, Unix ms.
	FoundAt int64 `json:"foundAt"`
	// Read is whether the user has read it (opened it, or marked it).
	Read bool `json:"read"`
}

// NewReleases is the tracker's list: newest release first (releases still to
// come lead), with how many are unread, and when the last complete check
// finished.
type NewReleases struct {
	Items     []NewRelease `json:"items"`
	Unread    int          `json:"unread"`
	CheckedAt int64        `json:"checkedAt"` // Unix ms; 0 before the first complete check
}
