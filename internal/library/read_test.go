package library

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

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
	artists, _ := e.lib.Artists(ctx, ListOptions{})
	if len(artists) != 2 || artists[0].Name != "abba" {
		t.Fatalf("artists = %+v", artists)
	}
	if hits, _ := e.lib.Search(ctx, `"*) OR`, 5); len(hits) != 0 {
		t.Fatalf("FTS syntax not neutralised: %+v", hits)
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
