package app

import (
	"errors"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		url  string
		want Target
	}{
		{"https://music.apple.com/us/album/abbey-road-remastered/1441164426",
			Target{Kind: KindAlbum, ID: "1441164426", Storefront: "us"}},
		{"https://music.apple.com/gb/album/1441164426",
			Target{Kind: KindAlbum, ID: "1441164426", Storefront: "gb"}},
		{"https://music.apple.com/us/album/come-together/1441164426?i=1441164430",
			Target{Kind: KindSong, ID: "1441164430", AlbumID: "1441164426", Storefront: "us"}},
		{"https://music.apple.com/de/album/abbey-road/1441164426?i=1441164430&l=en-GB",
			Target{Kind: KindSong, ID: "1441164430", AlbumID: "1441164426", Storefront: "de"}},
		{"https://music.apple.com/us/song/come-together/1441164430",
			Target{Kind: KindSong, ID: "1441164430", Storefront: "us"}},
		{"https://music.apple.com/us/song/1441164430",
			Target{Kind: KindSong, ID: "1441164430", Storefront: "us"}},
		{"https://music.apple.com/us/artist/the-beatles/136975",
			Target{Kind: KindArtist, ID: "136975", Storefront: "us"}},
		{"https://music.apple.com/us/playlist/todays-hits/pl.f4d106fed2bd41149aaacabb233eb5eb",
			Target{Kind: KindPlaylist, ID: "pl.f4d106fed2bd41149aaacabb233eb5eb", Storefront: "us"}},
		{"https://music.apple.com/us/playlist/pl.u-AbCdEf012345",
			Target{Kind: KindPlaylist, ID: "pl.u-AbCdEf012345", Storefront: "us"}},
		{"https://music.apple.com/library/playlist/p.ZyXwVu987654",
			Target{Kind: KindLibraryPlaylist, ID: "p.ZyXwVu987654"}},
		{"https://music.apple.com/us/library/playlist/p.ZyXwVu987654",
			Target{Kind: KindLibraryPlaylist, ID: "p.ZyXwVu987654", Storefront: "us"}},
		{"https://music.apple.com/library/albums/l.Qr5St7U",
			Target{Kind: KindLibraryAlbum, ID: "l.Qr5St7U"}},
		{"https://geo.music.apple.com/us/album/_/1441164426?i=1441164430&app=music",
			Target{Kind: KindSong, ID: "1441164430", AlbumID: "1441164426", Storefront: "us"}},
		{"https://embed.music.apple.com/us/album/abbey-road/1441164426",
			Target{Kind: KindAlbum, ID: "1441164426", Storefront: "us"}},
		{"https://itunes.apple.com/us/album/abbey-road/id1441164426?i=1441164430&uo=4",
			Target{Kind: KindSong, ID: "1441164430", AlbumID: "1441164426", Storefront: "us"}},
		{"itms://itunes.apple.com/us/album/abbey-road/id1441164426",
			Target{Kind: KindAlbum, ID: "1441164426", Storefront: "us"}},
		{"itmss://music.apple.com/us/artist/the-beatles/136975",
			Target{Kind: KindArtist, ID: "136975", Storefront: "us"}},
		{"music://music.apple.com/us/album/abbey-road/1441164426?i=1441164430",
			Target{Kind: KindSong, ID: "1441164430", AlbumID: "1441164426", Storefront: "us"}},
		{"musics://music.apple.com/jp/playlist/pl.f4d106fed2bd41149aaacabb233eb5eb",
			Target{Kind: KindPlaylist, ID: "pl.f4d106fed2bd41149aaacabb233eb5eb", Storefront: "jp"}},
		{"  https://MUSIC.apple.com/US/album/x/1441164426/  ",
			Target{Kind: KindAlbum, ID: "1441164426", Storefront: "us"}},
		{"https://music.apple.com/us/music-video/come-together/1442907386",
			Target{Kind: KindMusicVideo, ID: "1442907386", Storefront: "us"}},
		{"https://music.apple.com/us/station/the-beatles-station/ra.u-1d3c3a5bd1e22e4eb31aef1ab0d0d9b9",
			Target{Kind: KindStation, ID: "ra.u-1d3c3a5bd1e22e4eb31aef1ab0d0d9b9", Storefront: "us"}},
		{"https://music.apple.com/gb/station/apple-music-1/ra.978194965",
			Target{Kind: KindStation, ID: "ra.978194965", Storefront: "gb"}},
		{"https://music.apple.com/us/curator/apple-music-pop/976439548",
			Target{Kind: KindCurator, ID: "976439548", Storefront: "us"}},
		{"https://music.apple.com/us/apple-curator/apple-music/1526756058",
			Target{Kind: KindCurator, ID: "1526756058", Storefront: "us"}},
	}
	for _, tt := range tests {
		got, err := ParseURL(tt.url)
		if err != nil {
			t.Errorf("ParseURL(%q): %v", tt.url, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseURL(%q) = %+v, want %+v", tt.url, got, tt.want)
		}
	}
}

func TestParseURLRejects(t *testing.T) {
	tests := []struct {
		url         string
		unsupported bool
	}{
		{"https://music.apple.com/us/room/6449178938", true},
		{"https://music.apple.com/us/station/x/1442907386", false},
		{"https://music.apple.com/us/browse", true},
		{"https://music.apple.com/us", true},
		{"https://example.com/us/album/x/1441164426", true},
		{"https://music.apple.com.evil.example/us/album/x/1441164426", true},
		{"ftp://music.apple.com/us/album/x/1441164426", true},
		{"https://music.apple.com/library/songs/i.abc", true},
		{"https://music.apple.com/us/album/x/abc", false},
		{"https://music.apple.com/us/album/x/1441164426?i=zz", false},
		{"https://music.apple.com/us/playlist/x/1441164426", false},
		{"https://music.apple.com/us/playlist/pl.a<b>", false},
		{"://nope", false},
	}
	for _, tt := range tests {
		got, err := ParseURL(tt.url)
		if err == nil {
			t.Errorf("ParseURL(%q) = %+v, want error", tt.url, got)
			continue
		}
		if errors.Is(err, ErrUnsupportedURL) != tt.unsupported {
			t.Errorf("ParseURL(%q): %v, unsupported=%v", tt.url, err, tt.unsupported)
		}
	}
}

func TestIsMusicURL(t *testing.T) {
	for arg, want := range map[string]bool{
		"music://music.apple.com/us/album/x/1": true,
		"itms://itunes.apple.com/us/album/id1": true,
		"https://music.apple.com/us/album/x/1": true,
		"https://example.com/us/album/x/1":     false,
		"--debug":                              false,
		"/home/u/song.m4a":                     false,
	} {
		if got := IsMusicURL(arg); got != want {
			t.Errorf("IsMusicURL(%q) = %v, want %v", arg, got, want)
		}
	}
}
