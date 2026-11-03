package tunein

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"
)

// playURL is the shape of the play URL Apple returns for a TuneIn
// station: TuneIn's playlist, the station in its ad parameters.
func playURL(host, ad string) string {
	q := url.Values{"ti_type": {"mp3"}, "ti_ad_params": {ad}, "url": {"http://stream.example/live"}}
	return "https://" + host + "/pre/v1/playlist.pls?" + q.Encode()
}

func TestStationID(t *testing.T) {
	const ad = "aw_0_1st.playerid=Partner&aw_0_1st.bundleId=tunein.com&aw_0_1st.stationId=s100001&aw_0_1st.class=music"
	for _, c := range []struct {
		name, url, want string
	}{
		{"tunein playlist", playURL("pls.tunein.com", ad), "s100001"},
		{"other tunein host", playURL("stream.tunein.com", ad), "s100001"},
		// As Apple sends it: the ad parameters escaped a second time.
		{"escaped twice", playURL("pls.tunein.com", url.QueryEscape(ad)), "s100001"},
		{"not tunein", playURL("pls.example.com", ad), ""},
		{"look-alike host", playURL("pls.tunein.com.example.net", ad), ""},
		{"no station in the ad parameters", playURL("pls.tunein.com", "aw_0_1st.class=music"), ""},
		{"not a station ID", playURL("pls.tunein.com", "aw_0_1st.stationId=p100001"), ""},
		{"path injection", playURL("pls.tunein.com", "aw_0_1st.stationId=s1%2F..%2Fx"), ""},
		{"a broadcaster's own HLS", "http://hls.example/Station/playlist.m3u8", ""},
		{"not a URL", "://", ""},
	} {
		if got := StationID(c.url); got != c.want {
			t.Errorf("%s: StationID = %q, want %q", c.name, got, c.want)
		}
	}
}

// Describe.ashx's answers (render=json), with placeholder stations: the
// station Describe's HTTP 200 carries, with its faults in the head.
const (
	describeSong = ` { "head": { "status": "200"}, "body": [
{"element":"station","guide_id":"s100001","preset_id":"s100001","name":"Example FM","call_sign":"EX","slogan":"Music all day","frequency":"99.1","band":"FM","url":"https://www.example.com/","has_song":true,"has_schedule":false,"location":"Exampletown","current_song":"Sample Song","current_artist":"Sample Artist","current_album_art":null,"description":"  Hits from every decade.  ","genre_name":"Pop"}
] }`
	describeNoSong = ` { "head": { "status": "200"}, "body": [
{"element":"station","guide_id":"s100002","name":"Example Talk","slogan":"","url":"javascript:alert(1)","has_song":false,"location":"Exampletown","current_song":"Stale Song","current_artist":"Stale Artist","description":"Talk.","genre_name":"Talk"}
] }`
	describeFault = ` { "head": { "status": "400",
	"fault": "Station not found"}, "body": [
] }`
	describeEmpty = ` { "head": { "status": "200"}, "body": [
] }`
	describeOther = `{"head":{"status":"200"},"body":[{"element":"station","guide_id":"s999","name":"Someone Else"}]}`
)

func TestParseDescribe(t *testing.T) {
	got, err := parseDescribe("s100001", []byte(describeSong))
	if err != nil {
		t.Fatal(err)
	}
	want := Station{
		ID: "s100001", Name: "Example FM", Slogan: "Music all day", Description: "Hits from every decade.",
		Genre: "Pop", Location: "Exampletown", Website: "https://www.example.com/",
		HasSong: true, Artist: "Sample Artist", Title: "Sample Song",
	}
	if got != want {
		t.Fatalf("parseDescribe = %+v, want %+v", got, want)
	}

	// A station that does not feed TuneIn its songs has none, whatever
	// stale value its profile holds; a website that is no web page is
	// dropped.
	got, err = parseDescribe("s100002", []byte(describeNoSong))
	if err != nil {
		t.Fatal(err)
	}
	if got.HasSong || got.Artist != "" || got.Title != "" || got.Website != "" || got.Genre != "Talk" {
		t.Fatalf("no song: %+v", got)
	}

	for name, body := range map[string]string{"fault": describeFault, "empty": describeEmpty, "other station": describeOther} {
		if _, err := parseDescribe("s100001", []byte(body)); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := parseDescribe("s100001", []byte("<html>")); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("garbage: err = %v", err)
	}
}

// opml is a fake Describe.ashx answering from a script, one answer per
// request, the last repeating.
type opml struct {
	srv *httptest.Server

	mu     sync.Mutex
	script []string // body, or "HTTP <code>"
	n      int
	query  url.Values
	ua     string
}

func newOPML(t *testing.T, script ...string) *opml {
	o := &opml{script: script}
	o.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		defer o.mu.Unlock()
		o.query, o.ua = r.URL.Query(), r.UserAgent()
		body := o.script[min(o.n, len(o.script)-1)]
		o.n++
		if r.URL.Path != "/Describe.ashx" {
			http.NotFound(w, r)
			return
		}
		if body == "HTTP 503" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(o.srv.Close)
	return o
}

func (o *opml) client() *Client {
	return &Client{HTTP: o.srv.Client(), UserAgent: "Sona-test/1", Base: o.srv.URL}
}

func TestDescribe(t *testing.T) {
	o := newOPML(t, describeSong)
	s, err := o.client().Describe(context.Background(), "s100001")
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Sample Song" || s.Artist != "Sample Artist" {
		t.Fatalf("station = %+v", s)
	}
	if o.query.Get("id") != "s100001" || o.query.Get("render") != "json" || o.ua != "Sona-test/1" {
		t.Fatalf("request: %v %q", o.query, o.ua)
	}
	// Only a station ID reaches the URL.
	if _, err := o.client().Describe(context.Background(), "s1&c=x"); err == nil {
		t.Fatal("a bad ID was sent")
	}
	o = newOPML(t, "HTTP 503")
	if _, err := o.client().Describe(context.Background(), "s100001"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("503: %v", err)
	}
}

// pause records the waits Watch asks for and returns at once.
type pause struct {
	waits []time.Duration
	after func(n int) // runs after the n-th wait
}

func (p *pause) sleep(ctx context.Context, d time.Duration) error {
	p.waits = append(p.waits, d)
	if p.after != nil {
		p.after(len(p.waits))
	}
	return ctx.Err()
}

func TestWatchFollowsSongs(t *testing.T) {
	next := `{"head":{"status":"200"},"body":[{"element":"station","guide_id":"s100001","name":"Example FM","has_song":true,"current_song":"Second Song","current_artist":"Sample Artist"}]}`
	o := newOPML(t, describeSong, describeSong, next)
	var (
		got         []Station
		ctx, cancel = context.WithCancel(context.Background())
		p           = &pause{after: func(n int) {
			if n == 3 {
				cancel()
			}
		}}
	)
	defer cancel()
	w := &Watcher{Client: o.client(), ID: "s100001", Emit: func(s Station) { got = append(got, s) }, Sleep: p.sleep}
	if err := w.Watch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Watch = %v", err)
	}
	// An unchanged profile is not emitted twice.
	if len(got) != 2 || got[0].Title != "Sample Song" || got[1].Title != "Second Song" {
		t.Fatalf("emitted %+v", got)
	}
	if want := []time.Duration{SongInterval, SongInterval, SongInterval}; !slices.Equal(p.waits, want) {
		t.Fatalf("waits = %v, want %v", p.waits, want)
	}
}

func TestWatchBacksOffAndRecovers(t *testing.T) {
	o := newOPML(t, "HTTP 503", "HTTP 503", "HTTP 503", "HTTP 503", "HTTP 503", "HTTP 503", "HTTP 503", "HTTP 503", describeNoSong, "HTTP 503", describeNoSong)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &pause{after: func(n int) {
		if n == 11 {
			cancel()
		}
	}}
	emitted := 0
	w := &Watcher{Client: o.client(), ID: "s100002", Emit: func(Station) { emitted++ }, Sleep: p.sleep}
	_ = w.Watch(ctx)
	want := []time.Duration{
		30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		10 * time.Minute, 10 * time.Minute, 10 * time.Minute, // the cap
		ProfileInterval, // recovered: a station without songs is read rarely
		FailureBase,     // and the next failure starts the backoff over
		ProfileInterval,
	}
	if !slices.Equal(p.waits, want) {
		t.Fatalf("waits = %v\nwant    %v", p.waits, want)
	}
	if emitted != 1 {
		t.Fatalf("emitted %d times, want once (the second read changed nothing)", emitted)
	}
}

func TestWatchStops(t *testing.T) {
	// An unknown station is not retried.
	o := newOPML(t, describeFault)
	p := &pause{}
	w := &Watcher{Client: o.client(), ID: "s100001", Emit: func(Station) { t.Error("emitted") }, Sleep: p.sleep}
	if err := w.Watch(context.Background()); err != nil || len(p.waits) != 0 || o.n != 1 {
		t.Fatalf("not found: err %v, waits %v, requests %d", err, p.waits, o.n)
	}

	// Once the stream carries the song itself, the profile read at the
	// start is all it costs.
	o = newOPML(t, describeSong)
	p = &pause{}
	wanted := true
	p.after = func(int) { wanted = false }
	w = &Watcher{Client: o.client(), ID: "s100001", Emit: func(Station) {}, Want: func() bool { return wanted }, Sleep: p.sleep}
	if err := w.Watch(context.Background()); err != nil || o.n != 1 || len(p.waits) != 1 {
		t.Fatalf("not wanted: err %v, waits %v, requests %d", err, p.waits, o.n)
	}
}
