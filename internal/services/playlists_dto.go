package services

import "github.com/wailsapp/wails/v3/pkg/application"

// Playlists events.
const (
	// EventPlaylistsChanged: library playlists, the folder tree or the pins
	// changed on this machine (an edit) or after a refresh from Apple.
	EventPlaylistsChanged = "playlists:changed" // PlaylistsChanged
)

func init() {
	application.RegisterEvent[PlaylistsChanged](EventPlaylistsChanged)
}

// PlaylistsChanged is the "playlists:changed" payload.
type PlaylistsChanged struct {
	// IDs are the library playlists whose details or tracks changed.
	IDs []string `json:"ids"`
	// Tree: playlists or folders were added, removed, renamed or moved.
	Tree bool `json:"tree"`
	// Pins: the pins changed.
	Pins bool `json:"pins"`
}

// PlaylistCurator is who made a playlist: an Apple Music curator or show
// ("apple-curator"), an external curator ("curator"), or a user's social
// profile ("profile"). Kind is "" when unknown.
type PlaylistCurator struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Handle string `json:"handle"` // profiles
	URL    string `json:"url"`    // music.apple.com page
}

// PlaylistDetail is a playlist page: the playlist with every track in
// playlist order, and what the page shows around it.
type PlaylistDetail struct {
	Playlist   Playlist `json:"playlist"`
	Tracks     []Song   `json:"tracks"`
	DurationMs int64    `json:"durationMs"` // sum of the tracks
	// InLibrary: the playlist is in the user's library. For a catalog
	// playlist found there, Playlist.LibraryID is the library copy's ID ("" right
	// after it was added, until the sync stores it).
	InLibrary bool   `json:"inLibrary"`
	Header    Header `json:"header"`
	// Kind is Apple's playlistType for catalog playlists ("editorial",
	// "user-shared", "personal-mix", "replay", "external", …) and "library"
	// for a library playlist.
	Kind    string          `json:"kind"`
	IsChart bool            `json:"isChart"`
	Curator PlaylistCurator `json:"curator"`
	// FeaturedArtists and MoreByCurator: catalog playlists only.
	FeaturedArtists []Artist   `json:"featuredArtists"`
	MoreByCurator   []Playlist `json:"moreByCurator"`
	// Library playlists: what the user may do, the Favourite Songs tag and
	// the folder the playlist sits in ("" at the top).
	CanDelete bool   `json:"canDelete"`
	IsPublic  bool   `json:"isPublic"`
	Favorited bool   `json:"favorited"`
	FolderID  string `json:"folderId"`
	// URL is the playlist's music.apple.com page ("" for private library
	// playlists).
	URL string `json:"url"`
}

// CollaborationRole is what the signed-in user is to a collaborative
// playlist.
type CollaborationRole string

const (
	RoleOwner        CollaborationRole = "owner"        // started it
	RoleCollaborator CollaborationRole = "collaborator" // was let in to add songs
	RoleListener     CollaborationRole = "listener"     // neither, or not known
)

// PlaylistCollaboration is who works on a collaborative playlist, from
// Apple (network). Collaborative is false for every other playlist, and
// then the rest is empty.
type PlaylistCollaboration struct {
	Collaborative bool `json:"collaborative"`
	// OwnerID is the owner's profile ID, one of Collaborators ("" when
	// Apple names none).
	OwnerID string `json:"ownerId"`
	// Collaborators are everyone who can add songs, the owner first.
	Collaborators []SocialProfile `json:"collaborators"`
	// Former are people who added songs but no longer collaborate.
	Former []SocialProfile    `json:"former"`
	Viewer CollaborationRole `json:"viewer"`
	// AddedBy says who added each of the playlist's first 300 tracks, in
	// Apple's order (later tracks are not attributed).
	AddedBy []TrackAddedBy `json:"addedBy"`
}

// TrackAddedBy is one track of a collaborative playlist: its catalog ID
// and the profile ID of who added it ("" when Apple does not say).
type TrackAddedBy struct {
	SongID    string `json:"songId"`
	ProfileID string `json:"profileId"`
}

// PlaylistInput is the editable part of a library playlist.
type PlaylistInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"isPublic"`
}

// PlaylistItem is something to add to a playlist: a song, or every song of
// an album or playlist. Kind is "song", "album" or "playlist"; ID is in
// Source.
type PlaylistItem struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Source Source `json:"source"`
}

// PlaylistFolder is a folder of the user's playlist tree. ParentID is ""
// at the top; Position orders siblings (folders and playlists together).
type PlaylistFolder struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

// TreePlaylist is a library playlist placed in the tree.
type TreePlaylist struct {
	Playlist  Playlist `json:"playlist"`
	FolderID  string   `json:"folderId"`
	Position  int      `json:"position"`
	CanDelete bool     `json:"canDelete"`
	Favorited bool     `json:"favorited"`
}

// PlaylistTree is the user's folders and library playlists, from SQLite.
type PlaylistTree struct {
	Folders   []PlaylistFolder `json:"folders"`
	Playlists []TreePlaylist   `json:"playlists"`
}

// Pin is one sidebar pin. Kind is "song", "album", "artist" or "playlist";
// ID is the item's library ID. Action is what activating it does: "play",
// "shuffle" or "drillIn" (open its page). Song is set for song pins;
// AlbumID is a song pin's library album.
type Pin struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Subtitle string `json:"subtitle"`
	Action   string `json:"action"`
	Art      Art    `json:"art"`
	Song     Song   `json:"song"`
	AlbumID  string `json:"albumId"`
}
