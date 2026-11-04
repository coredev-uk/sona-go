// Package fixture is Sona's synthetic library: invented artists, albums and
// songs, and generated covers, with no data from Apple or from any account.
// It seeds a sona.db (Seed) for the UI tests (internal/ui/uitest), the e2e
// smoke (e2e) and the component gallery, and serves the covers the way
// Apple's artwork CDN would (CDN). Everything is a pure function of the
// index of the album, so a golden image or a test's expectation stays put.
package fixture

import (
	"fmt"
	"slices"
)

// Album is one album of the library; Songs of it are stored too.
type Album struct {
	Name, Artist string
	Single       bool
	Explicit     bool
	// Titles are the songs' names; nil names them "<Name> song <n>", two of
	// them.
	Titles []string
}

// SongCount is how many songs the album has.
func (a Album) SongCount() int {
	if len(a.Titles) == 0 {
		return 2
	}
	return len(a.Titles)
}

// Song is the name of the album's j-th song (from 0).
func (a Album) Song(j int) string {
	if j < len(a.Titles) {
		return a.Titles[j]
	}
	return fmt.Sprintf("%s song %d", a.Name, j+1)
}

// The identifiers a seeded library gives album i (from 0, in the order
// seeded) and its songs.
func AlbumLibraryID(i int) string    { return fmt.Sprintf("l.a%d", i) }
func AlbumCatalogID(i int) string    { return fmt.Sprint(100 + i) }
func SongLibraryID(i, j int) string  { return fmt.Sprintf("i.a%dt%d", i, j) }
func SongCatalogID(i, j int) string  { return fmt.Sprintf("%d%d", 1000+i, j) }
func ArtistLibraryID(n int) string   { return fmt.Sprintf("r.a%d", n) }
func PlaylistLibraryID(n int) string { return fmt.Sprintf("p.a%d", n) }

// Playlist is a user playlist holding songs of the albums by index.
type Playlist struct {
	Name        string
	Description string
	// Songs are (album, song) pairs.
	Songs [][2]int
}

// Albums is the library the UI tests, the e2e smoke and the golden images
// use: 24 albums by 12 invented artists, three of them singles, three
// explicit.
func Albums() []Album {
	titles := [][]string{
		{"Low Tide Radio", "Paper Lanterns", "Salt and Lemon", "Quiet Machines"},
		{"The Long Way Round", "Dust on the Dial", "Copper Wire"},
		{"Honey Static", "Midnight Bakery", "Slow Orbit", "Greenhouse"},
		{"Weather Report for Two", "Small Fires", "Harbour Lights"},
		{"Satellite Garden", "Velvet Hours", "Cold Brew", "Northbound", "Blue Spruce"},
		{"Afterglow Avenue", "Wool and Wire", "Tin Roof"},
	}
	var out []Album
	for i, a := range []struct {
		name, artist string
		single, expl bool
	}{
		{"Glass Harbour", "Nova Ketter", false, false},
		{"Paper Moons", "The Lantern Choir", false, false},
		{"Static Honey", "Mira Oyelaran", false, true},
		{"Slow Orbit", "Hollis & Vane", false, false},
		{"Greenhouse Radio", "Juniper Call", false, false},
		{"Copper Wire", "Tomas Ekwueme", true, false},
		{"Midnight Bakery", "Saffron Quay", false, false},
		{"Weather for Two", "Nova Ketter", false, false},
		{"Small Fires", "The Marlow Set", false, true},
		{"Harbour Lights", "Ines Calder", false, false},
		{"Satellite Garden", "Wren Atlas", false, false},
		{"Velvet Hours", "Hollis & Vane", false, false},
		{"Cold Brew", "Dara Finch", true, false},
		{"Northbound", "The Lantern Choir", false, false},
		{"Blue Spruce", "Juniper Call", false, false},
		{"Afterglow Avenue", "Kofi Bellamy", false, false},
		{"Wool and Wire", "Saffron Quay", false, true},
		{"Tin Roof Sessions", "The Marlow Set", false, false},
		{"Quiet Machines", "Ines Calder", false, false},
		{"Salt and Lemon", "Mira Oyelaran", true, false},
		{"Dust on the Dial", "Tomas Ekwueme", false, false},
		{"The Long Way Round", "Wren Atlas", false, false},
		{"Low Tide", "Dara Finch", false, false},
		{"Fieldnotes", "Kofi Bellamy", false, false},
	} {
		out = append(out, Album{Name: a.name, Artist: a.artist, Single: a.single, Explicit: a.expl, Titles: titles[i%len(titles)][:2+i%2]})
	}
	return out
}

// Playlists are the library's user playlists over Albums.
func Playlists() []Playlist {
	return []Playlist{
		{Name: "Road trip", Description: "Windows down.", Songs: [][2]int{{0, 0}, {3, 1}, {5, 0}, {8, 1}, {11, 0}, {14, 1}}},
		{Name: "Focus", Songs: [][2]int{{1, 1}, {4, 0}, {7, 1}, {10, 0}}},
		{Name: "Sunday morning", Description: "Coffee first.", Songs: [][2]int{{2, 0}, {6, 1}, {9, 0}, {12, 1}, {15, 0}}},
	}
}

// Artists are the distinct artists of albums, in order of first appearance.
func Artists(albums []Album) []string {
	var out []string
	for _, a := range albums {
		if !slices.Contains(out, a.Artist) {
			out = append(out, a.Artist)
		}
	}
	return out
}
