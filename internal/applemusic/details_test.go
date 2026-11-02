package applemusic

import (
	"encoding/json"
	"net/http"
	"testing"
)

// The fixtures below are trimmed from live amp-api responses (storefront gb).

func TestAlbumPage(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/catalog/gb/albums/7":
			if q.Get("platform") != "web" || q.Get("views") != "more-by-artist,you-might-also-like,appears-on,other-versions,related-videos,audio-extras" ||
				q.Get("meta[albums:tracks]") != "popularity" || q.Get("include") != "artists,genres" || q.Get("fields[genres]") != "name,parentId" {
				t.Errorf("album query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"7","type":"albums","attributes":{"name":"Big","isPrerelease":false,"editorialNotes":{"short":"Short.","standard":"A <i>long</i> note."},
				"artwork":{"url":"https://a/{w}x{h}bb.jpg","bgColor":"cccbc9","textColor1":"0e0808","textColor2":"392926","textColor3":"342f2f","textColor4":"564a47"}},
				"relationships":{"artists":{"data":[{"id":"55","type":"artists","attributes":{"name":"Band"}}]},
				"genres":{"data":[{"id":"5","type":"genres","attributes":{"name":"Classical","parentId":"34"}},{"id":"34","type":"genres","attributes":{"name":"Music"}}]},
				"tracks":{"data":[{"id":"1","type":"songs","meta":{"popularity":0.67},"attributes":{"name":"Goldberg Variations: Aria","discNumber":1,"composerName":"Johann Sebastian Bach","workName":"Goldberg Variations","movementName":"Aria"}}],"next":"/v1/catalog/gb/albums/7/tracks?offset=1"}},
				"views":{"more-by-artist":{"attributes":{"title":"More by Band"},"data":[{"id":"8","type":"albums","attributes":{"name":"Other"}}],"next":"/v1/catalog/gb/albums/7/view/more-by-artist?offset=15"},
				"appears-on":{"attributes":{"title":"Featured On"},"data":[{"id":"pl.1","type":"playlists","attributes":{"name":"Mix","curatorName":"Apple Music"}}]},
				"related-videos":{"attributes":{"title":"Music Videos"},"data":[{"id":"9","type":"music-videos","attributes":{"name":"Clip","has4K":true,"durationInMillis":254612}}]},
				"audio-extras":{"attributes":{"title":"More to Hear"},"data":[{"id":"ra.1745191891","type":"stations","attributes":{"name":"No. 11: Rumours","kind":"streaming","isLive":false,"durationInMillis":1800000}}]},
				"other-versions":{"attributes":{"title":"Other Versions"},"data":[]}},
				"meta":{"views":{"order":["other-versions","related-videos","more-by-artist","appears-on","you-might-also-like","audio-extras"]}}}]}`))
		case "/v1/catalog/gb/albums/7/tracks":
			if q.Get("offset") != "1" || q.Get("limit") != "100" || q.Get("platform") != "web" || q.Get("include[songs]") != "artists" {
				t.Errorf("tracks query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"id":"2","type":"songs","meta":{"popularity":0.2},"attributes":{"name":"Two","discNumber":2}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base
	a, err := AlbumPage(t.Context(), c, "gb", "7")
	if err != nil {
		t.Fatal(err)
	}
	tr := a.Relationships.Tracks.Data
	if a.Attributes.Name != "Big" || len(tr) != 2 || tr[1].Attributes.Name != "Two" || tr[0].Meta.Popularity != 0.67 {
		t.Fatalf("album = %+v", a)
	}
	v := a.Views
	if len(v.MoreByArtist.Data) != 1 || v.MoreByArtist.Next == "" || v.AppearsOn.Data[0].Attributes.CuratorName != "Apple Music" ||
		!v.RelatedVideos.Data[0].Attributes.Has4K || len(v.OtherVersions.Data) != 0 || v.AudioExtras.Data[0].ID != "ra.1745191891" {
		t.Fatalf("views = %+v", v)
	}
	if art := a.Attributes.Artwork; art.BgColor != "cccbc9" || art.TextColor1 != "0e0808" || art.TextColor3 != "342f2f" || art.TextColor4 != "564a47" {
		t.Fatalf("artwork colours = %+v", art)
	}
	if g := a.Relationships.Genres.Data; len(g) != 2 || g[1].Attributes.Name != "Music" || g[1].Attributes.ParentID != "" {
		t.Fatalf("genres = %+v", g)
	}
	if s := tr[0].Attributes; s.WorkName != "Goldberg Variations" || s.MovementName != "Aria" || s.ComposerName != "Johann Sebastian Bach" {
		t.Fatalf("classical attributes = %+v", s)
	}
	if got := Sized(a.Attributes.Artwork.Template(), 300, 300); got != "https://a/300x300bb.jpg" {
		t.Fatalf("Sized = %s", got)
	}
	// The typed resource round-trips through JSON (the catalog cache relies
	// on it), and empty views are not written.
	b, _ := json.Marshal(a)
	var back Album
	if err := json.Unmarshal(b, &back); err != nil || back.ID != "7" || len(back.Relationships.Tracks.Data) != 2 ||
		back.Views.RelatedVideos.Data[0].ID != "9" || back.Relationships.Tracks.Data[0].Meta.Popularity != 0.67 {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	b, _ = json.Marshal(back.Views.MoreByArtist.Data[0])
	var raw map[string]any
	json.Unmarshal(b, &raw)
	if _, ok := raw["views"]; ok {
		t.Errorf("empty views serialised: %s", b)
	}
}

func TestArtistPageAndView(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/v1/catalog/gb/artists/657515":
			if q.Get("extend") != "artistBio,bornOrFormed,origin,editorialVideo" || q.Get("platform") != "web" {
				t.Errorf("artist query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"id":"657515","type":"artists","attributes":{"name":"Radiohead","genreNames":["Alternative"],"origin":"Abingdon, Oxfordshire, England","bornOrFormed":"1985","artistBio":"More than a band.<br/>Art."},
				"views":{"latest-release":{"attributes":{"title":"Latest Release"},"data":[{"id":"1","type":"albums","attributes":{"name":"Hail"}}]},
				"top-songs":{"attributes":{"title":"Top Songs"},"data":[{"id":"2","type":"songs","attributes":{"name":"Creep"},"relationships":{"albums":{"data":[{"id":"3","type":"albums"}]}}}],"next":"/v1/catalog/gb/artists/657515/view/top-songs?offset=10"},
				"singles":{"attributes":{"title":"Singles & EPs"},"data":[{"id":"4","type":"albums","attributes":{"name":"Single","isSingle":true}}]},
				"similar-artists":{"attributes":{"title":"Similar Artists"},"data":[{"id":"5","type":"artists","attributes":{"name":"Muse"}}]},
				"music-videos":{"attributes":{"title":"Music Videos"},"data":[{"id":"6","type":"music-videos","attributes":{"name":"Clip"}}]},
				"playlists":{"attributes":{"title":"Artist Playlists"},"data":[{"id":"pl.7","type":"playlists","attributes":{"name":"Essentials"}}]}}}]}`))
		case "/v1/catalog/gb/artists/657515/view/top-songs":
			if q.Get("offset") != "20" || q.Get("limit") != "100" || q.Get("with") != "attributes" {
				t.Errorf("view query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"attributes":{"title":"Top Songs"},"data":[{"id":"9","type":"songs","attributes":{"name":"Nude"}}],"next":"/v1/catalog/gb/artists/657515/view/top-songs?offset=21"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base
	a, err := ArtistPage(t.Context(), c, "gb", "657515")
	if err != nil {
		t.Fatal(err)
	}
	at, v := a.Attributes, a.Views
	if at.Origin == "" || at.BornOrFormed != "1985" || at.ArtistBio == "" || v.LatestRelease.Data[0].ID != "1" ||
		v.TopSongs.Data[0].Relationships.Albums.Data[0].ID != "3" || NextOffset(v.TopSongs.Next) != 10 ||
		!v.Singles.Data[0].Attributes.IsSingle || v.SimilarArtists.Data[0].Attributes.Name != "Muse" ||
		v.MusicVideos.Data[0].ID != "6" || v.Playlists.Data[0].ID != "pl.7" || len(v.FullAlbums.Data) != 0 {
		t.Fatalf("artist = %+v", a)
	}
	page, err := ArtistView[Song](t.Context(), c, "gb", "657515", ViewTopSongs, 20, 500)
	if err != nil || len(page.Data) != 1 || page.Data[0].Attributes.Name != "Nude" || page.Attributes.Title != "Top Songs" || NextOffset(page.Next) != 21 {
		t.Fatalf("view = %+v, %v", page, err)
	}
	if NextOffset("") != -1 || NextOffset("/v1/x?limit=2") != -1 {
		t.Fatal("NextOffset without offset")
	}
}

func TestSongPageCredits(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/gb/songs/1109715168" || r.URL.Query().Get("include") != "albums,artists,credits,music-videos" ||
			r.URL.Query().Get("extend") != "lyricsExcerpt" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"data":[{"id":"1109715168","type":"songs","attributes":{"name":"Weird Fishes / Arpeggi","hasTimeSyncedLyrics":true,"lyricsExcerpt":"In the deepest ocean\nThe bottom of the sea"},
			"relationships":{"albums":{"data":[{"id":"1109714933","type":"albums","attributes":{"name":"In Rainbows"}}]},
			"credits":{"data":[{"id":"1109715168-4","type":"role-categories","attributes":{"kind":"performer","title":"PERFORMING ARTISTS"},
				"relationships":{"credit-artists":{"data":[{"id":"4-ami:identity:x","type":"credit-artists","attributes":{"name":"Thom Yorke","roleNames":["Guitar","Lead Vocals"]}}]}}}]},
			"music-videos":{"data":[]}}}]}`))
	}))
	c.AMPBase = c.Base
	s, err := SongPage(t.Context(), c, "gb", "1109715168")
	if err != nil {
		t.Fatal(err)
	}
	cr := s.Relationships.Credits.Data
	if !s.Attributes.HasTimeSyncedLyrics || s.Attributes.LyricsExcerpt == "" || len(cr) != 1 || cr[0].Attributes.Kind != "performer" ||
		cr[0].Relationships.CreditArtists.Data[0].Attributes.RoleNames[1] != "Lead Vocals" || s.Relationships.Albums.Data[0].ID != "1109714933" {
		t.Fatalf("song = %+v", s)
	}
}

func TestMusicVideoPage(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/gb/music-videos/1447440393" || r.URL.Query().Get("views") != "more-by-artist,more-in-genre" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"data":[{"id":"1447440393","type":"music-videos","attributes":{"name":"Bodysnatchers","artistName":"Radiohead","durationInMillis":254612,"has4K":false,"hasHDR":true,"genreNames":["Alternative"],"url":"https://music.apple.com/gb/music-video/x/1447440393"},
			"relationships":{"artists":{"data":[{"id":"657515","type":"artists","attributes":{"name":"Radiohead"}}]},"songs":{"data":[{"id":"1109715161","type":"songs","attributes":{"name":"Bodysnatchers"}}]}},
			"views":{"more-by-artist":{"attributes":{"title":"More by Radiohead"},"data":[{"id":"2","type":"music-videos","attributes":{"name":"Other"}}]},"more-in-genre":{"attributes":{"title":"You Might Also Like"},"data":[]}}}]}`))
	}))
	c.AMPBase = c.Base
	v, err := MusicVideoPage(t.Context(), c, "gb", "1447440393")
	if err != nil {
		t.Fatal(err)
	}
	if v.CatalogID() != "1447440393" || !v.Attributes.HasHDR || v.Relationships.Songs.Data[0].ID != "1109715161" ||
		len(v.Views.MoreByArtist.Data) != 1 || len(v.Views.MoreInGenre.Data) != 0 {
		t.Fatalf("video = %+v", v)
	}
	var lib MusicVideo
	json.Unmarshal([]byte(`{"id":"i.9","type":"library-music-videos","attributes":{"playParams":{"catalogId":"99"}}}`), &lib)
	if lib.CatalogID() != "99" {
		t.Fatalf("library CatalogID = %q", lib.CatalogID())
	}
}
