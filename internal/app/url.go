package app

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// Kind classifies what an Apple Music link points at.
type Kind string

const (
	KindSong            Kind = "song"
	KindAlbum           Kind = "album"
	KindArtist          Kind = "artist"
	KindPlaylist        Kind = "playlist"         // catalog playlist, ID "pl.…"
	KindLibraryPlaylist Kind = "library-playlist" // user's playlist, ID "p.…"
	KindLibraryAlbum    Kind = "library-album"    // user's album, ID "l.…"
	KindMusicVideo      Kind = "music-video"
	KindStation         Kind = "station" // ID "ra.…"
	KindCurator         Kind = "curator" // curators and Apple curators
)

// Target is a parsed Apple Music link.
type Target struct {
	Kind Kind
	ID   string
	// AlbumID is set for KindSong when the link was an album link with
	// ?i={songID} (the form Apple uses to share a song in its album).
	AlbumID string
	// Storefront is the two-letter storefront in the path ("us"), or "" for
	// links without one (library links).
	Storefront string
}

// ErrUnsupportedURL is wrapped by ParseURL for well-formed Apple Music links
// Sona cannot open (rooms, browse pages, library songs, …) and for anything
// that is not an Apple Music link.
var ErrUnsupportedURL = errors.New("unsupported Apple Music URL")

// schemes are the URL schemes ParseURL accepts. music/musics/itms/itmss are
// what Apple Music and iTunes links use when handed to a desktop handler.
var schemes = map[string]bool{
	"https": true, "http": true,
	"music": true, "musics": true,
	"itms": true, "itmss": true,
}

// hosts are the Apple hosts that serve music links.
var hosts = map[string]bool{
	"music.apple.com":       true,
	"geo.music.apple.com":   true,
	"embed.music.apple.com": true,
	"beta.music.apple.com":  true,
	"itunes.apple.com":      true,
	"geo.itunes.apple.com":  true,
}

// IsMusicURL reports whether arg looks like a link ParseURL should be given:
// a music/musics/itms/itmss URL, or an http(s) URL on an Apple music host.
// Use it to tell URLs from other command-line arguments.
func IsMusicURL(arg string) bool {
	u, err := url.Parse(arg)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme == "http" || scheme == "https" {
		return hosts[strings.ToLower(u.Hostname())]
	}
	return schemes[scheme]
}

// ParseURL converts an Apple Music link into a Target. It understands
// music.apple.com paths of the forms
//
//	/{sf}/album/{slug}/{id}[?i={songID}]   → album, or song within album
//	/{sf}/song/{slug}/{id}                 → song
//	/{sf}/artist/{slug}/{id}               → artist
//	/{sf}/playlist/{slug}/pl.{id}          → catalog playlist
//	/{sf}/music-video/{slug}/{id}          → music video
//	/{sf}/station/{slug}/ra.{id}           → station
//	/{sf}/[apple-]curator/{slug}/{id}      → curator
//	[/{sf}]/library/playlist/p.{id}        → library playlist
//	[/{sf}]/library/albums/l.{id}          → library album
//
// where the slug is optional, legacy iTunes IDs ("id1441164426") are
// accepted, and the scheme may be https, http, music, musics, itms or itmss.
// Other link types wrap ErrUnsupportedURL.
func ParseURL(raw string) (Target, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Target{}, fmt.Errorf("parse %q: %w", raw, err)
	}
	if !schemes[strings.ToLower(u.Scheme)] || !hosts[strings.ToLower(u.Hostname())] {
		return Target{}, fmt.Errorf("%w: %q is not an Apple Music link", ErrUnsupportedURL, raw)
	}
	var segs []string
	for s := range strings.SplitSeq(u.Path, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	var t Target
	if len(segs) > 0 && isStorefront(segs[0]) {
		t.Storefront = strings.ToLower(segs[0])
		segs = segs[1:]
	}
	if len(segs) < 2 {
		return Target{}, fmt.Errorf("%w: %q has no item", ErrUnsupportedURL, raw)
	}
	kind, id := segs[0], segs[len(segs)-1]
	if len(segs) > 3 {
		return Target{}, fmt.Errorf("%w: %q", ErrUnsupportedURL, raw)
	}

	switch kind {
	case "album", "song", "artist", "music-video", "curator", "apple-curator":
		num, ok := catalogID(id)
		if !ok {
			return Target{}, fmt.Errorf("invalid %s id %q in %q", kind, id, raw)
		}
		t.Kind, t.ID = Kind(kind), num
		if kind == "apple-curator" {
			t.Kind = KindCurator
		}
		if kind == "album" {
			if song := u.Query().Get("i"); song != "" {
				songID, ok := catalogID(song)
				if !ok {
					return Target{}, fmt.Errorf("invalid song id %q in %q", song, raw)
				}
				t.Kind, t.ID, t.AlbumID = KindSong, songID, num
			}
		}
		return t, nil
	case "station":
		if !strings.HasPrefix(id, "ra.") || !validID(id) {
			return Target{}, fmt.Errorf("invalid station id %q in %q", id, raw)
		}
		t.Kind, t.ID = KindStation, id
		return t, nil
	case "playlist":
		if !strings.HasPrefix(id, "pl.") || !validID(id) {
			return Target{}, fmt.Errorf("invalid playlist id %q in %q", id, raw)
		}
		t.Kind, t.ID = KindPlaylist, id
		return t, nil
	case "library":
		if len(segs) != 3 {
			break
		}
		switch {
		case segs[1] == "playlist" && strings.HasPrefix(id, "p.") && validID(id):
			t.Kind, t.ID = KindLibraryPlaylist, id
			return t, nil
		case segs[1] == "albums" && strings.HasPrefix(id, "l.") && validID(id):
			t.Kind, t.ID = KindLibraryAlbum, id
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("%w: %q", ErrUnsupportedURL, raw)
}

// isStorefront reports whether s is a two-letter storefront code.
func isStorefront(s string) bool {
	return len(s) == 2 && isLetter(s[0]) && isLetter(s[1])
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// catalogID accepts a numeric catalog ID, optionally with the legacy iTunes
// "id" prefix, and returns the bare number.
func catalogID(s string) (string, bool) {
	s = strings.TrimPrefix(s, "id")
	if s == "" {
		return "", false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
	}
	return s, true
}

// validID accepts the characters Apple uses in non-numeric IDs.
func validID(s string) bool {
	for i := range len(s) {
		c := s[i]
		if !isLetter(c) && (c < '0' || c > '9') && c != '.' && c != '-' && c != '_' {
			return false
		}
	}
	return len(s) > 3
}
