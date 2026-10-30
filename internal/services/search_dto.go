package services

// Search result groups beyond Apple's catalog groups (applemusic.Group*):
// the social search's profiles and user-shared playlists.
const (
	GroupProfiles        = "profile"
	GroupSharedPlaylists = "shared_playlist"
)

// ItemProfile is an Item kind for an Apple Music user profile (search
// only); /profile/$id.
const ItemProfile = "profile"

// Recent search kinds besides item kinds.
const RecentTerm = "term"

// Search scopes.
const (
	ScopeCatalog = "catalog"
	ScopeLibrary = "library"
)

// SearchSection is one group of Apple Music results, in Apple's order.
// Group is "top" (mixed kinds), an applemusic.Group* key ("artist",
// "album", "song", "playlist", "radio_show", "radio_episode", "station",
// "music_video", …), GroupProfiles or GroupSharedPlaylists. Next is the
// offset of the group's next page for SearchService.Group, 0 when the
// group has no more.
type SearchSection struct {
	Group string `json:"group"`
	Items []Item `json:"items"`
	Next  int    `json:"next"`
}

// SearchResults are the Apple Music results for Term.
type SearchResults struct {
	Term     string          `json:"term"`
	Sections []SearchSection `json:"sections"`
}

// SearchGroupPage is one page of a single result group ("see all").
type SearchGroupPage struct {
	Group string `json:"group"`
	Items []Item `json:"items"`
	Next  int    `json:"next"` // 0: last page
}

// SearchTerm is a term completion: Term is what to search for, Display how
// to show it.
type SearchTerm struct {
	Term    string `json:"term"`
	Display string `json:"display"`
}

// SearchHints are the suggestions for a partial term: completions and the
// top results to open directly.
type SearchHints struct {
	Terms []SearchTerm `json:"terms"`
	Items []Item       `json:"items"`
}

// LibrarySearch is the library scope of a search: up to the requested
// number of matches per kind, best first; *More flags further matches.
type LibrarySearch struct {
	Songs         []Song     `json:"songs"`
	Albums        []Album    `json:"albums"`
	Artists       []Artist   `json:"artists"`
	Playlists     []Playlist `json:"playlists"`
	SongsMore     bool       `json:"songsMore"`
	AlbumsMore    bool       `json:"albumsMore"`
	ArtistsMore   bool       `json:"artistsMore"`
	PlaylistsMore bool       `json:"playlistsMore"`
}

// RecentSearch is a remembered search: a term (Kind RecentTerm) searched
// in Scope ("catalog" or "library"), or an item opened from search (Kind
// is the item's kind, ItemID its ID in Source). Key identifies the entry
// for SearchService.ForgetRecent.
type RecentSearch struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Scope      string `json:"scope"` // terms only
	Term       string `json:"term"`
	ItemID     string `json:"itemId"`
	Source     Source `json:"source"` // items only
	Name       string `json:"name"`
	Subtitle   string `json:"subtitle"`
	Art        string `json:"art"` // Go-served small artwork URL, "" for terms
	SearchedAt int64  `json:"searchedAt"`
}

// RecentItem is an item opened from search, to remember (SearchService.RememberItem).
type RecentItem struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Source   Source `json:"source"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Art      string `json:"art"` // the Go-served URL the result showed
}

// SocialProfile is an Apple Music user profile.
type SocialProfile struct {
	ID       string `json:"id"` // "sp.…"
	Name     string `json:"name"`
	Handle   string `json:"handle"`
	Verified bool   `json:"verified"`
	Private  bool   `json:"private"`
	URL      string `json:"url"`
	Art      Art    `json:"art"`
}

// ProfileDetail is a profile page: the profile and its public playlists.
type ProfileDetail struct {
	Profile   SocialProfile `json:"profile"`
	Playlists []Playlist    `json:"playlists"`
}
