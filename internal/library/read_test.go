package library

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

func TestListSorting(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Zed"), artist("r.b", "abba")}
	f.albums = []map[string]any{
		album("l.1", "beta", "Zed", "r.a", "2024-01-02T00:00:00Z"),
		album("l.2", "Alpha", "abba", "r.b", "2024-03-01T00:00:00Z"),
	}
	f.songs = []map[string]any{ // newest first
		song("i.1", "charlie", "Zed", "beta", "l.1", "r.a"),
		song("i.2", "Bravo", "abba", "Alpha", "l.2", "r.b"),
		song("i.3", "alpha", "Zed", "Alpha", "l.2", "r.a"),
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ids := func(s []Song) (out string) {
		for _, x := range s {
			out += x.LibraryID + " "
		}
		return out
	}
	for _, tc := range []struct {
		o    ListOptions
		want string
	}{
		{ListOptions{}, "i.3 i.2 i.1 "}, // title, case-insensitive
		{ListOptions{Desc: true}, "i.1 i.2 i.3 "},
		{ListOptions{Sort: SortArtist}, "i.2 i.3 i.1 "},
		{ListOptions{Sort: SortAlbum}, "i.2 i.3 i.1 "},
		{ListOptions{Sort: SortDateAdded, Desc: true}, "i.1 i.2 i.3 "},
		{ListOptions{Sort: SortDateAdded}, "i.3 i.2 i.1 "},
		{ListOptions{Limit: 1, Offset: 1}, "i.2 "},
	} {
		got, err := e.lib.Songs(ctx, tc.o)
		if err != nil || ids(got) != tc.want {
			t.Errorf("Songs(%+v) = %q, %v; want %q", tc.o, ids(got), err, tc.want)
		}
	}
	albums, _ := e.lib.Albums(ctx, ListOptions{Sort: SortDateAdded, Desc: true})
	if len(albums) != 2 || albums[0].LibraryID != "l.2" {
		t.Fatalf("albums by date = %+v", albums)
	}
	albums, _ = e.lib.Albums(ctx, ListOptions{})
	if albums[0].Name != "Alpha" {
		t.Fatalf("albums by title = %+v", albums)
	}
	artists, _ := e.lib.ArtistIndex(ctx)
	if len(artists) != 2 || artists[0].Name != "abba" {
		t.Fatalf("artists = %+v", artists)
	}
	if hits, _ := e.lib.Search(ctx, `"*) OR`, 5); len(hits) != 0 {
		t.Fatalf("FTS syntax not neutralised: %+v", hits)
	}
}

func TestArtistIndex(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	bare := artist("r.c", "Étienne")
	delete(bare["relationships"].(map[string]any), "catalog") // an artist Apple gives no artwork
	f.artists = []map[string]any{artist("r.a", "Zed"), artist("r.b", "abba"), bare, artist("r.d", "Nobody")}
	old := album("l.1", "Beta", "Zed", "r.a", "2024-01-02T00:00:00Z")
	old["attributes"].(map[string]any)["releaseDate"] = "2010-05-01"
	cover := album("l.3", "Gamma", "Étienne", "r.c", "2024-02-01T00:00:00Z")
	cover["attributes"].(map[string]any)["artwork"] = map[string]any{"url": "https://art/gamma/{w}x{h}bb.jpg"}
	f.albums = []map[string]any{
		old,
		album("l.2", "Alpha", "Zed", "r.a", "2024-03-01T00:00:00Z"),
		cover,
	}
	f.songs = []map[string]any{
		song("i.1", "One", "Zed", "Beta", "l.1", "r.a"),
		song("i.2", "Two", "Zed", "Alpha", "l.2", "r.a"),
		song("i.3", "Three", "Zed", "Alpha", "l.2", "r.a"),
		song("i.4", "Four", "abba", "Elsewhere", "l.x", "r.b"), // songs without an album in the library
	}
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := e.lib.ArtistIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]ArtistSummary{}
	var order []string
	for _, a := range got {
		byID[a.LibraryID] = a
		order = append(order, a.LibraryID)
	}
	// Counts are the library's albums and songs. Case never decides the
	// order (accents are the UI's to fold: SQLite's NOCASE is ASCII only).
	counts := map[string][2]int{"r.a": {2, 3}, "r.b": {0, 1}, "r.c": {1, 0}, "r.d": {0, 0}}
	for id, want := range counts {
		if a, ok := byID[id]; !ok || [2]int{a.Albums, a.Songs} != want {
			t.Errorf("artist %s albums, songs = %d, %d (listed %v), want %v", id, a.Albums, a.Songs, ok, want)
		}
	}
	if len(got) != len(counts) || slices.Index(order, "r.b") > slices.Index(order, "r.d") || slices.Index(order, "r.d") > slices.Index(order, "r.a") {
		t.Errorf("artists in order %v, want abba, Nobody, Zed (r.b r.d r.a)", order)
	}
	if a := byID["r.c"]; a.Albums != 1 || a.ArtworkTemplate != "https://art/gamma/{w}x{h}bb.jpg" {
		t.Errorf("an artist without artwork takes their album's cover: %+v", a)
	}
	if a := byID["r.a"]; a.ArtworkTemplate == "" {
		t.Errorf("an artist's own artwork is kept: %+v", a)
	}

	a, albums, err := e.lib.ArtistAlbums(ctx, "r.a")
	if err != nil || a.Name != "Zed" || len(albums) != 2 || albums[0].LibraryID != "l.2" || albums[1].LibraryID != "l.1" {
		t.Fatalf("ArtistAlbums = %+v, %+v, %v; want Zed's albums newest release first", a, albums, err)
	}
	if _, _, err := e.lib.ArtistAlbums(ctx, "r.nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ArtistAlbums(unknown) err = %v, want ErrNotFound", err)
	}
}

// TestLargeLibraryListsFast syncs a 12k-song library through the real sync
// path and checks that the UI's list, count and search queries stay fast
// as a library grows. Their wall time is too noisy to check on shared CI
// runners under -race (the benchmarks below measure it), so the test
// checks what makes a query slow instead: each call runs a fixed number
// of statements, reads no more rows than it needs, and no statement scans
// a table once per row of another (a time that grows with the square of
// the library).
func TestLargeLibraryListsFast(t *testing.T) {
	if testing.Short() {
		t.Skip("large library")
	}
	e := newEnv(t)
	seed(e.fake, 12000)
	for i := range 3000 { // a library of thousands of artists, most with one song or none
		e.fake.artists = append(e.fake.artists, artist("r.big"+strconv.Itoa(i), "Band "+strconv.Itoa(i)))
	}
	start := time.Now()
	if err := e.run(t, DomainArtists, DomainAlbums, DomainSongs); err != nil {
		t.Fatal(err)
	}
	t.Logf("synced 12000 songs, 1200 albums in %v", time.Since(start).Round(time.Millisecond))
	ctx := context.Background()
	lib, reads := recordReads(t, e)
	// check runs f, which must return want results from statements
	// statements reading at most rows rows in all.
	check := func(name string, f func() (int, error), want, statements, rows int) {
		t.Helper()
		reads.reset()
		n, err := f()
		if err != nil || n != want {
			t.Fatalf("%s: %d results, %v", name, n, err)
		}
		ran := reads.done()
		read := 0
		for _, s := range ran {
			read += s.rows
			if table := scannedPerRow(s.plan); table != "" {
				t.Errorf("%s scans %s once per row of another loop:\n%s\n%s", name, table, s.query, s.plan)
			}
		}
		t.Logf("%-28s %d statements, %d rows read", name, len(ran), read)
		if len(ran) != statements || read > rows {
			t.Errorf("%s ran %d statements reading %d rows, want %d reading at most %d", name, len(ran), read, statements, rows)
		}
	}
	for _, s := range []Sort{SortTitle, SortArtist, SortAlbum, SortDateAdded} {
		for _, off := range []int{0, 11900} {
			check("songs "+string(s)+" @"+strconv.Itoa(off), func() (int, error) {
				r, err := lib.Songs(ctx, ListOptions{Sort: s, Offset: off, Limit: 100})
				return len(r), err
			}, 100, 1, 100)
		}
	}
	check("albums title", func() (int, error) {
		r, err := lib.Albums(ctx, ListOptions{Limit: 200})
		return len(r), err
	}, 200, 1, 200)
	check("counts", func() (int, error) {
		c, err := lib.Counts(ctx)
		return c.Songs, err
	}, 12000, 1, 4) // a row per kind
	check("search prefix", func() (int, error) {
		r, err := lib.Search(ctx, "son 1199", 50)
		return len(r), err
	}, 11, 1, 11)
	check("artist index", func() (int, error) {
		r, err := lib.ArtistIndex(ctx)
		return len(r), err
	}, 3005, 1, 3005)
	check("artist albums", func() (int, error) {
		_, r, err := lib.ArtistAlbums(ctx, "r.1")
		return len(r), err
	}, 240, 2, 241) // the artist, then their albums
	check("album detail", func() (int, error) {
		_, r, err := lib.Album(ctx, "l.500")
		return len(r), err
	}, 10, 2, 11)
}

// readLog records the statements run on a recordReads pool.
type readLog struct {
	mu    sync.Mutex
	stmts []*readStmt
}

type readStmt struct {
	query string
	plan  queryPlan
	rows  int // read so far
}

func (l *readLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stmts = nil
}

func (l *readLog) done() []readStmt {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]readStmt, len(l.stmts))
	for i, s := range l.stmts {
		out[i] = *s
	}
	return out
}

// recordReads returns a Library over e's database whose read pool records
// every query it runs, with its plan and the rows read.
func recordReads(t *testing.T, e *env) (*Library, *readLog) {
	t.Helper()
	log := &readLog{}
	dsn := "file:" + (&url.URL{Path: e.path}).EscapedPath() + "?_busy_timeout=5000&_query_only=1"
	r := sql.OpenDB(recordingConnector{drv: e.db.R.Driver(), dsn: dsn, log: log})
	t.Cleanup(func() { _ = r.Close() })
	return New(&db.DB{W: e.db.W, Q: e.db.Q, R: r, RQ: dbq.New(r)}, nil), log
}

type recordingConnector struct {
	drv driver.Driver
	dsn string
	log *readLog
}

func (c recordingConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := c.drv.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	q, ok := conn.(driver.QueryerContext)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("%T runs no queries without preparing them", conn)
	}
	return recordingConn{Conn: conn, q: q, log: c.log}, nil
}

func (c recordingConnector) Driver() driver.Driver { return c.drv }

// recordingConn is a read connection that records its queries. Library
// only queries on its read pool, so the rest is the plain driver.Conn.
type recordingConn struct {
	driver.Conn
	q   driver.QueryerContext
	log *readLog
}

func (c recordingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	s := &readStmt{query: query}
	plan, err := c.q.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, args)
	if err != nil {
		return nil, err
	}
	row := make([]driver.Value, len(plan.Columns()))
	for plan.Next(row) == nil {
		id, _ := row[0].(int64)
		parent, _ := row[1].(int64)
		s.plan = append(s.plan, planStep{id: id, parent: parent, detail: fmt.Sprint(row[3])})
	}
	if err := plan.Close(); err != nil {
		return nil, err
	}
	rows, err := c.q.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	c.log.mu.Lock()
	c.log.stmts = append(c.log.stmts, s)
	c.log.mu.Unlock()
	return &countingRows{Rows: rows, log: c.log, s: s}, nil
}

type countingRows struct {
	driver.Rows
	log *readLog
	s   *readStmt
}

func (r *countingRows) Next(dest []driver.Value) error {
	err := r.Rows.Next(dest)
	if err == nil {
		r.log.mu.Lock()
		r.s.rows++
		r.log.mu.Unlock()
	}
	return err
}

// queryPlan is a statement's EXPLAIN QUERY PLAN: a tree of steps, each
// loop (SCAN or SEARCH) nested in the loops before it under one parent.
type queryPlan []planStep

type planStep struct {
	id, parent int64
	detail     string
}

func (p queryPlan) String() string {
	var b strings.Builder
	for _, s := range p {
		fmt.Fprintf(&b, "%d %d %s\n", s.id, s.parent, s.detail)
	}
	return b.String()
}

// scannedPerRow returns what p scans in full once per row of an outer
// loop (or of the outer query, in a correlated subquery), if anything.
func scannedPerRow(p queryPlan) string {
	var walk func(parent int64, perRow bool) string
	walk = func(parent int64, perRow bool) string {
		inLoop := perRow
		for _, s := range p {
			if s.parent != parent {
				continue
			}
			if table, ok := strings.CutPrefix(s.detail, "SCAN "); ok && inLoop {
				return table
			}
			if strings.HasPrefix(s.detail, "SCAN ") || strings.HasPrefix(s.detail, "SEARCH ") {
				inLoop = true
			}
			if table := walk(s.id, perRow || strings.HasPrefix(s.detail, "CORRELATED ")); table != "" {
				return table
			}
		}
		return ""
	}
	return walk(0, false)
}

func benchEnv(b *testing.B, n int) *env {
	b.Helper()
	e := newEnv(b)
	seed(e.fake, n)
	if err := e.run(b, DomainArtists, DomainAlbums, DomainSongs); err != nil {
		b.Fatal(err)
	}
	return e
}

func BenchmarkSongsPage(b *testing.B) {
	e := benchEnv(b, 10000)
	ctx := context.Background()
	for _, s := range []Sort{SortTitle, SortArtist, SortDateAdded} {
		b.Run(string(s), func(b *testing.B) {
			for i := 0; b.Loop(); i++ {
				if _, err := e.lib.Songs(ctx, ListOptions{Sort: s, Offset: (i * 100) % 10000, Limit: 100}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSearch(b *testing.B) {
	e := benchEnv(b, 10000)
	ctx := context.Background()
	for b.Loop() {
		if _, err := e.lib.Search(ctx, "son 12", 50); err != nil {
			b.Fatal(err)
		}
	}
}

// flagged returns library album a with isSingle on its included catalog album.
func flagged(a map[string]any, isSingle bool) map[string]any {
	c := a["relationships"].(map[string]any)["catalog"].(map[string]any)["data"].([]any)[0].(map[string]any)
	c["attributes"] = map[string]any{"isSingle": isSingle}
	return a
}

func TestIsSingleRelease(t *testing.T) {
	named := func(name string) applemusic.Album {
		var a applemusic.Album
		a.Attributes.Name = name
		return a
	}
	withCatalog := func(name string, isSingle bool) applemusic.Album {
		a := named(name)
		var c applemusic.Album
		c.Attributes.IsSingle = isSingle
		a.Relationships.Catalog.Data = []applemusic.Album{c}
		return a
	}
	for _, tc := range []struct {
		why  string
		in   applemusic.Album
		want bool
	}{
		{"single suffix", named("Night Drive - Single"), true},
		{"EP suffix", named("Night Drive - EP"), true},
		{"catalog flag, other language", withCatalog("Night Drive - シングル", true), true},
		{"catalog flag without suffix", withCatalog("Night Drive", true), true},
		{"suffix wins over an unflagged catalog album", withCatalog("Night Drive - EP", false), true},
		{"standard album", withCatalog("Night Drive", false), false},
		{"no catalog album, no suffix", named("Night Drive"), false},
		{"title that merely contains the words", named("Single - Ladies"), false},
		{"lower-case suffix is not Apple's", named("night drive - ep"), false},
	} {
		if got := IsSingleRelease(tc.in); got != tc.want {
			t.Errorf("%s: IsSingleRelease = %v, want %v", tc.why, got, tc.want)
		}
	}
}

// TestAlbumTypeFilter syncs albums of every kind through the real path and
// checks the type filter with each sort, paging and the count.
func TestAlbumTypeFilter(t *testing.T) {
	e := newEnv(t)
	f := e.fake
	f.artists = []map[string]any{artist("r.a", "Zed"), artist("r.b", "abba")}
	f.albums = []map[string]any{ // newest first
		album("l.1", "Dawn - Single", "Zed", "r.a", "2024-05-01T00:00:00Z"),
		flagged(album("l.2", "Dusk", "abba", "r.b", "2024-04-01T00:00:00Z"), true), // flag only
		album("l.3", "Cargo", "Zed", "r.a", "2024-03-01T00:00:00Z"),
		album("l.4", "Atlas - EP", "abba", "r.b", "2024-02-01T00:00:00Z"),
		flagged(album("l.5", "Beacon", "Zed", "r.a", "2024-01-01T00:00:00Z"), false),
	}
	f.albums[0]["attributes"].(map[string]any)["releaseDate"] = "2024-04-30"
	f.albums[2]["attributes"].(map[string]any)["releaseDate"] = "2019-03-01"
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	names := func(o ListOptions) string {
		t.Helper()
		as, err := e.lib.Albums(ctx, o)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, a := range as {
			out = append(out, a.Name)
		}
		return strings.Join(out, ", ")
	}
	for _, tc := range []struct {
		o    ListOptions
		want string
	}{
		{ListOptions{}, "Atlas - EP, Beacon, Cargo, Dawn - Single, Dusk"},
		{ListOptions{Type: AlbumTypeAlbum}, "Beacon, Cargo"},
		{ListOptions{Type: AlbumTypeSingle}, "Atlas - EP, Dawn - Single, Dusk"},
		{ListOptions{Type: AlbumTypeSingle, Desc: true}, "Dusk, Dawn - Single, Atlas - EP"},
		{ListOptions{Type: AlbumTypeSingle, Sort: SortArtist}, "Atlas - EP, Dusk, Dawn - Single"},
		{ListOptions{Type: AlbumTypeSingle, Sort: SortDateAdded, Desc: true}, "Dawn - Single, Dusk, Atlas - EP"},
		{ListOptions{Type: AlbumTypeAlbum, Sort: SortReleaseDate}, "Cargo, Beacon"},
		{ListOptions{Type: AlbumTypeSingle, Limit: 2, Offset: 1}, "Dawn - Single, Dusk"},
		{ListOptions{Type: AlbumTypeAlbum, Limit: 5, Offset: 2}, ""},
		{ListOptions{Type: "nonsense"}, "Atlas - EP, Beacon, Cargo, Dawn - Single, Dusk"},
	} {
		if got := names(tc.o); got != tc.want {
			t.Errorf("Albums(%+v) = %q, want %q", tc.o, got, tc.want)
		}
	}
	for typ, want := range map[AlbumType]int{AlbumTypeAll: 5, AlbumTypeAlbum: 2, AlbumTypeSingle: 3} {
		if n, err := e.lib.AlbumCount(ctx, typ); err != nil || n != want {
			t.Errorf("AlbumCount(%q) = %d, %v; want %d", typ, n, err, want)
		}
	}

	// Every album read (here a library album's page) carries the flag too.
	for id, want := range map[string]bool{"l.1": true, "l.2": true, "l.3": false, "l.4": true, "l.5": false} {
		if a, _, err := e.lib.Album(ctx, id); err != nil || a.Single != want {
			t.Errorf("Album(%s).Single = %v, %v; want %v", id, a.Single, err, want)
		}
	}

	// A catalog flag Apple changes later is picked up by the next full run.
	f.albums[2] = flagged(album("l.3", "Cargo", "Zed", "r.a", "2024-03-01T00:00:00Z"), true)
	if err := e.runFull(t); err != nil {
		t.Fatal(err)
	}
	if got := names(ListOptions{Type: AlbumTypeAlbum}); got != "Beacon" {
		t.Errorf("after the flag changed, albums = %q", got)
	}
}

func TestAllPagesCollectsEveryRow(t *testing.T) {
	for _, n := range []int{0, 1, maxPage - 1, maxPage, maxPage + 1, 2*maxPage + 5} {
		var calls int
		got, err := allPages(ListOptions{Offset: 3}, func(o ListOptions) ([]int, error) {
			calls++
			if o.Limit != maxPage {
				t.Fatalf("n=%d: Limit = %d, want %d", n, o.Limit, maxPage)
			}
			var page []int
			for i := o.Offset; i < min(o.Offset+o.Limit, n+3); i++ {
				page = append(page, i)
			}
			return page, nil
		})
		if err != nil || len(got) != n {
			t.Fatalf("n=%d: %d rows, %v", n, len(got), err)
		}
		if want := n/maxPage + 1; calls != want {
			t.Errorf("n=%d: %d calls, want %d", n, calls, want)
		}
	}
	boom := errors.New("boom")
	if _, err := allPages(ListOptions{}, func(ListOptions) ([]int, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
}
