package services

// Discover events.
const (
	// EventHome carries the new HomePage after a background refresh.
	EventHome = "discover:home"
)

// HomePage is Home: Apple's personal recommendation shelves in order.
// Sections is empty when signed out. FetchedAt is when Apple produced them
// (Unix ms, 0 when never); Stale marks a cached copy past its refresh time
// that could not be renewed.
type HomePage struct {
	Sections  []Section `json:"sections"`
	FetchedAt int64     `json:"fetchedAt"`
	Stale     bool      `json:"stale"`
}

// RoomPage is an editorial room (a section's "See all" grid): its first
// page of items. Kind is SectionShelf or SectionTracks. Sorts lists the
// orders Apple offers ("featured", "alphabet", "releaseDate"); Sort is the
// one in use. Next is the offset of the next page (0: no more).
type RoomPage struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Kind  string   `json:"kind"`
	Items []Item   `json:"items"`
	Sorts []string `json:"sorts"`
	Sort  string   `json:"sort"`
	Next  int      `json:"next"`
}

// ItemPage is a further page of a room or chart; Next as in RoomPage.
type ItemPage struct {
	Items []Item `json:"items"`
	Next  int    `json:"next"`
}

// Genre is a chart genre; ID "" is all music.
type Genre struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Chart kinds (Chart.Key).
const (
	ChartSongs       = "songs"
	ChartAlbums      = "albums"
	ChartPlaylists   = "playlists"
	ChartMusicVideos = "music-videos"
	ChartCities      = "cityCharts"           // top 25 by city (playlists)
	ChartDailyGlobal = "dailyGlobalTopCharts" // daily top 100 by country (playlists)
)

// Chart is one chart's first page. ChartsService pages it with Key,
// ChartID and the genre.
type Chart struct {
	Key     string `json:"key"`
	ChartID string `json:"chartId"` // city and daily charts only
	Title   string `json:"title"`
	Items   []Item `json:"items"`
	Next    int    `json:"next"`
}

// ChartsPage is Charts for one genre (Genre "" is all music), charts in
// Apple's order.
type ChartsPage struct {
	Genre     string  `json:"genre"`
	Genres    []Genre `json:"genres"`
	Charts    []Chart `json:"charts"`
	FetchedAt int64   `json:"fetchedAt"`
	Stale     bool    `json:"stale"`
}
