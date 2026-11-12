package artwork

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// imageServer stands in for is*-ssl.mzstatic.com: every request, whatever
// its host, is dialled to an httptest TLS server that renders a noisy
// red/blue image for the box named in the path ("…/512x256bb.jpg"): a
// JPEG, a PNG for ".png" paths, or a TIFF payload for ".tif" paths. Like
// Apple's CDN, an image whose path holds "/native-<w>x<h>/" is fitted into
// the box for "bb" and cropped to a w×w square for "cc"; any other image is
// rendered at the box.
type imageServer struct {
	srv    *httptest.Server
	mu     sync.Mutex
	hits   map[string]int
	agents map[string]string // path → User-Agent of the last request
	delay  time.Duration
	// entered, when set, gets a value as each request arrives; gate, when
	// set, holds every request until a value is sent (or the gate closes) or
	// the client gives up, which counts in cancelled.
	entered   chan struct{}
	gate      chan struct{}
	cancelled chan struct{}
	// notReady answers this many requests for "/gen/" covers with 406, as
	// Apple's CDN does for a new playlist's cover while it renders. Like
	// Apple's generated-artwork origin, "/gen/" covers are also refused with
	// 406 when the request's Accept has no */*.
	notReady int
}

func newImageServer(t *testing.T) *imageServer {
	s := &imageServer{hits: map[string]int{}, agents: map[string]string{}}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *imageServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.hits[r.Host+r.URL.Path]++
	s.agents[r.URL.Path] = r.UserAgent()
	s.mu.Unlock()
	time.Sleep(s.delay)
	if s.entered != nil {
		s.entered <- struct{}{}
	}
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-r.Context().Done():
			if s.cancelled != nil {
				s.cancelled <- struct{}{}
			}
			return
		}
	}
	if strings.Contains(r.URL.Path, "/gen/") {
		if !strings.Contains(r.Header.Get("Accept"), "*/*") {
			http.Error(w, "", http.StatusNotAcceptable)
			return
		}
		s.mu.Lock()
		wait := s.notReady > 0
		s.notReady--
		s.mu.Unlock()
		if wait {
			http.Error(w, "", http.StatusNotAcceptable)
			return
		}
	}
	if strings.Contains(r.URL.Path, "redirect") {
		http.Redirect(w, r, "https://evil.example/x.jpg", http.StatusFound)
		return
	}
	var bw, bh int
	base := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	if _, err := fmt.Sscanf(base, "%dx%d", &bw, &bh); err != nil || bw <= 0 || bh <= 0 {
		http.NotFound(w, r)
		return
	}
	iw, ih := bw, bh
	var nw, nh int
	if i := strings.Index(r.URL.Path, "/native-"); i >= 0 {
		_, _ = fmt.Sscanf(r.URL.Path[i+len("/native-"):], "%dx%d", &nw, &nh)
	}
	code := strings.TrimLeft(base, "0123456789x")
	switch {
	case nw > 0 && strings.HasPrefix(code, "bb"):
		s := min(float64(bw)/float64(nw), float64(bh)/float64(nh), 1)
		iw, ih = max(1, int(float64(nw)*s+0.5)), max(1, int(float64(nh)*s+0.5))
	case nw > 0 && strings.HasPrefix(code, "cc"):
		ih = bw
	}
	rng := rand.New(rand.NewPCG(uint64(iw*ih), uint64(len(r.URL.Path))))
	img := image.NewRGBA(image.Rect(0, 0, iw, ih))
	// An image whose path holds "/solid-rrggbb/" is that one colour.
	var solid color.RGBA
	if i := strings.Index(r.URL.Path, "/solid-"); i >= 0 {
		var rgb uint32
		_, _ = fmt.Sscanf(r.URL.Path[i+len("/solid-"):i+len("/solid-")+6], "%06x", &rgb)
		solid = color.RGBA{uint8(rgb >> 16), uint8(rgb >> 8), uint8(rgb), 255}
	}
	for y := range ih {
		for x := range iw {
			j := uint8(rng.IntN(40))
			switch {
			case solid.A != 0:
				img.Set(x, y, solid)
			case x < iw/2:
				img.Set(x, y, color.RGBA{200 + j/2, 20 + j, 20 + j, 255})
			default:
				img.Set(x, y, color.RGBA{20 + j, 50 + j, 200 + j/2, 255})
			}
		}
	}
	switch {
	case strings.HasSuffix(base, ".png"):
		w.Header().Set("Content-Type", "image/png")
		_ = png.Encode(w, img)
	case strings.HasSuffix(base, ".tif"):
		w.Header().Set("Content-Type", "image/tiff")
		_, _ = w.Write(tiffPayload)
	default:
		w.Header().Set("Content-Type", "image/jpeg")
		_ = jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
	}
}

func (s *imageServer) count(host, path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[host+path]
}

func (s *imageServer) total() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, v := range s.hits {
		n += v
	}
	return n
}

func (s *imageServer) client() *http.Client {
	addr := s.srv.Listener.Addr().String()
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
}

func newCache(t *testing.T, s *imageServer, dir string, maxBytes int64) *Cache {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	c, err := New(Options{Dir: dir, HTTP: s.client(), MaxBytes: maxBytes})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

const host = "is1-ssl.mzstatic.com"

func tmplFor(name string) string {
	return "https://" + host + "/image/thumb/Music/" + name + "/{w}x{h}bb.jpg"
}

func pathFor(name string, n int) string {
	return fmt.Sprintf("/image/thumb/Music/%s/%dx%dbb.jpg", name, n, n)
}

func bounds(t *testing.T, b []byte) image.Rectangle {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img.Bounds()
}

func TestVariantReuse(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	ctx := t.Context()
	tmpl := tmplFor("a")

	b, err := c.Get(ctx, tmpl, 512, 512)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 512 {
		t.Fatalf("512 variant is %v", got)
	}
	for _, size := range []int{128, 300, 512, 100} {
		b, err := c.Get(ctx, tmpl, size, size)
		if err != nil {
			t.Fatal(err)
		}
		if got := bounds(t, b); got.Dx() != size || got.Dy() != size {
			t.Fatalf("size %d: got %v", size, got)
		}
	}
	if n := s.total(); n != 1 {
		t.Fatalf("fetches = %d, want 1 (all sizes ≤ 512 from one source)", n)
	}
	if _, err := c.Get(ctx, tmpl, 700, 700); err != nil {
		t.Fatal(err)
	}
	if s.count(host, pathFor("a", 1024)) != 1 || s.total() != 2 {
		t.Fatalf("700 px should fetch the 1024 source once; hits %v", s.hits)
	}

	// A small request on a fresh template fetches the nearest standard size.
	if _, err := c.Get(ctx, tmplFor("b"), 40, 40); err != nil {
		t.Fatal(err)
	}
	if s.count(host, pathFor("b", 64)) != 1 {
		t.Fatalf("40 px should fetch 64 source; hits %v", s.hits)
	}
}

func TestSingleflight(t *testing.T) {
	s := newImageServer(t)
	s.delay = 50 * time.Millisecond
	c := newCache(t, s, "", 0)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Go(func() {
			size := 62 + i%3 // 62..64: one 64 px source
			_, err := c.Get(t.Context(), tmplFor("same"), size, size)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := s.total(); n != 1 {
		t.Fatalf("fetches = %d, want 1", n)
	}
}

func TestHostValidation(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	bad := []string{
		"",
		"http://is1-ssl.mzstatic.com/a/{w}x{h}bb.jpg",
		"https://evil.example/a/{w}x{h}bb.jpg",
		"https://mzstatic.com.evil.example/a/{w}x{h}bb.jpg",
		"https://evilmzstatic.com/a/{w}x{h}bb.jpg",
		"https://.mzstatic.com/a/{w}x{h}bb.jpg",
		"https://u:p@is1-ssl.mzstatic.com/a/{w}x{h}bb.jpg",
		"https://is1-ssl.mzstatic.com:8443/a/{w}x{h}bb.jpg",
		"file:///etc/passwd",
	}
	for _, tmpl := range bad {
		if u := c.URL(tmpl, 128, 128); u != "" {
			t.Errorf("URL(%q) = %q, want empty", tmpl, u)
		}
		if _, err := c.Get(t.Context(), tmpl, 128, 128); err == nil {
			t.Errorf("Get(%q) succeeded", tmpl)
		}
	}
	if _, err := c.Get(t.Context(), "https://evil.example/{w}x{h}.jpg", 64, 64); !errors.Is(err, ErrHost) {
		t.Errorf("want ErrHost, got %v", err)
	}
	if s.total() != 0 {
		t.Fatalf("rejected templates were fetched: %v", s.hits)
	}

	good := []string{
		"https://is1-ssl.mzstatic.com/image/thumb/x/{w}x{h}bb.jpg",
		"https://IS5-SSL.mzstatic.com/image/thumb/x/{w}x{h}{c}.{f}",
		"https://is2-ssl.mzstatic.com:443/image/thumb/x/600x600bb.jpg",
	}
	for _, tmpl := range good {
		if u := c.URL(tmpl, 128, 128); !strings.HasPrefix(u, "/art/") {
			t.Errorf("URL(%q) = %q", tmpl, u)
		}
	}

	// Redirects off the CDN are refused.
	if _, err := c.Get(t.Context(), tmplFor("redirect"), 64, 64); !errors.Is(err, ErrHost) {
		t.Fatalf("redirect: want ErrHost, got %v", err)
	}
	if s.count("evil.example", "/x.jpg") != 0 {
		t.Fatal("followed redirect to evil.example")
	}

	// Unregistered or malformed keys resolve to no template, so nothing is
	// ever fetched for them.
	for _, p := range []string{
		"/art/" + keyOf("https://evil.example/{w}x{h}.jpg") + "/64.jpg",
		"/art/not-a-key/64.jpg",
		"/art/../../etc/64.jpg",
	} {
		if tmpl, ok := c.Template(p); ok {
			t.Errorf("Template(%s) = %q, want none", p, tmpl)
		}
	}
}

func TestFixedTemplate(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Music/f/600x600bb.jpg"
	for _, size := range []int{100, 256} {
		b, err := c.Get(t.Context(), tmpl, size, size)
		if err != nil {
			t.Fatal(err)
		}
		if got := bounds(t, b); got.Dx() != size {
			t.Fatalf("size %d: %v", size, got)
		}
	}
	if s.total() != 1 {
		t.Fatalf("fetches = %d", s.total())
	}
}

func TestBackdropIsSmall(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	b, err := c.Backdrop(t.Context(), tmplFor("bd"))
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 64 || got.Dy() != 64 {
		t.Fatalf("backdrop is %v", got)
	}
	if len(b) > 8<<10 {
		t.Fatalf("backdrop is %d bytes", len(b))
	}
	if s.count(host, pathFor("bd", 64)) != 1 || s.total() != 1 {
		t.Fatalf("backdrop should fetch only the 64 px source: %v", s.hits)
	}
	// Blurring flattens the noise: neighbouring pixels differ little.
	img, _, _ := image.Decode(bytes.NewReader(b))
	r1, _, _, _ := img.At(5, 32).RGBA()
	r2, _, _, _ := img.At(6, 32).RGBA()
	if d := int(r1>>8) - int(r2>>8); d > 12 || d < -12 {
		t.Fatalf("backdrop not blurred: neighbouring red %d vs %d", r1>>8, r2>>8)
	}
}

func TestDominantColors(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	cols, err := c.DominantColors(t.Context(), tmplFor("dc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) < 2 || len(cols) > 3 {
		t.Fatalf("got %d swatches", len(cols))
	}
	var red, blue bool
	for _, col := range cols {
		red = red || (col.R > 150 && col.B < 100)
		blue = blue || (col.B > 150 && col.R < 100)
	}
	if !red || !blue {
		t.Fatalf("want a red and a blue swatch, got %v", cols)
	}
	if txt, _ := cols[0].MarshalText(); len(txt) != 7 || txt[0] != '#' {
		t.Fatalf("MarshalText = %q", txt)
	}
}

func TestURLRoundTrip(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	tmpl := tmplFor("h")
	u := c.URL(tmpl, 200, 200)
	if !strings.HasSuffix(u, "/200x200.jpg") {
		t.Fatalf("URL = %q", u)
	}
	if got, ok := c.Template(u + "?x=1#y"); !ok || got != tmpl {
		t.Fatalf("Template(%q) = %q, %v", u, got, ok)
	}
	b, err := c.Get(t.Context(), tmpl, 200, 200)
	if err != nil || bounds(t, b).Dx() != 200 {
		t.Fatalf("Get: %v", err)
	}
	if p, err := c.LocalPath(t.Context(), tmpl, 200, 200); err != nil || !strings.HasPrefix(p, dir) {
		t.Fatalf("LocalPath = %q, %v", p, err)
	}
	if b, err := c.Backdrop(t.Context(), tmpl); err != nil || len(b) == 0 {
		t.Fatalf("Backdrop: %d bytes, %v", len(b), err)
	}
	if got, ok := c.Template(c.BackdropURL(tmpl)); !ok || got != tmpl {
		t.Fatalf("Template(backdrop URL) = %q, %v", got, ok)
	}

	// After a restart the key resolves from disk without re-registering and
	// without refetching.
	before := s.total()
	c2 := newCache(t, s, dir, 0)
	got, ok := c2.Template(u)
	if !ok || got != tmpl {
		t.Fatalf("after restart: Template = %q, %v", got, ok)
	}
	if b2, err := c2.Get(t.Context(), got, 200, 200); err != nil || !bytes.Equal(b2, b) {
		t.Fatalf("after restart: Get: %v", err)
	}
	if s.total() != before {
		t.Fatal("restart refetched")
	}
}

func TestEviction(t *testing.T) {
	s := newImageServer(t)
	probe := newCache(t, s, "", 0)
	if _, err := probe.Get(t.Context(), tmplFor("probe"), 256, 256); err != nil {
		t.Fatal(err)
	}
	unit := probe.Size()

	dir := t.TempDir()
	limit := unit * 37 / 10
	c := newCache(t, s, dir, limit) // room for three keys, not four
	get := func(name string) {
		t.Helper()
		if _, err := c.Get(t.Context(), tmplFor(name), 256, 256); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, keyOf(tmplFor(name))))
		return err == nil
	}
	get("k1")
	get("k2")
	get("k3")
	get("k1") // k2 is now least recently used
	get("k4")
	if exists("k2") {
		t.Fatal("least recently used key survived")
	}
	for _, n := range []string{"k1", "k3", "k4"} {
		if !exists(n) {
			t.Fatalf("%s evicted", n)
		}
	}
	if c.Size() > limit {
		t.Fatalf("size %d over cap %d", c.Size(), limit)
	}

	// Evicted artwork refetches transparently.
	before := s.count(host, pathFor("k2", 256))
	get("k2")
	if s.count(host, pathFor("k2", 256)) != before+1 {
		t.Fatal("evicted key not refetched")
	}

	// A restart indexes what is on disk and enforces the cap.
	c2 := newCache(t, s, dir, unit*3/2)
	if c2.Size() > unit*3/2 {
		t.Fatalf("reopened size %d over cap %d", c2.Size(), unit*3/2)
	}
}

func TestClear(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	tmpl := tmplFor("c")
	if _, err := c.Get(t.Context(), tmpl, 128, 128); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if c.Size() != 0 {
		t.Fatalf("size after clear %d", c.Size())
	}
	if des, _ := os.ReadDir(dir); len(des) != 0 {
		t.Fatalf("dir not empty: %v", des)
	}
	// Registered URLs keep working.
	if _, err := c.Get(t.Context(), tmpl, 128, 128); err != nil || s.total() != 2 {
		t.Fatalf("after clear: %v, fetches %d", err, s.total())
	}
}

func TestSetMaxBytesEvictsAtOnce(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	for _, n := range []string{"m1", "m2", "m3", "m4"} {
		if _, err := c.Get(t.Context(), tmplFor(n), 256, 256); err != nil {
			t.Fatal(err)
		}
	}
	limit := c.Size() / 2
	c.SetMaxBytes(limit)
	if c.MaxBytes() != limit || c.Size() > limit {
		t.Fatalf("size %d, cap %d after lowering it to %d", c.Size(), c.MaxBytes(), limit)
	}
	if _, err := os.Stat(filepath.Join(dir, keyOf(tmplFor("m4")))); err != nil {
		t.Fatalf("most recently used key evicted: %v", err)
	}
	c.SetMaxBytes(0)
	if c.MaxBytes() != defaultMaxBytes {
		t.Fatalf("cap %d after reset", c.MaxBytes())
	}
}

func TestRemove(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	for _, n := range []string{"user", "url", "keep"} {
		if _, err := c.Get(t.Context(), tmplFor(n), 128, 128); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Backdrop(t.Context(), tmplFor("url")); err != nil {
		t.Fatal(err)
	}
	before := c.Size()
	freed, err := c.Remove([]string{tmplFor("user"), c.BackdropURL(tmplFor("url")), "https://evil.example/{w}x{h}.jpg", ""})
	if err != nil {
		t.Fatal(err)
	}
	if freed <= 0 || c.Size() != before-freed {
		t.Fatalf("freed %d, size %d → %d", freed, before, c.Size())
	}
	des, _ := os.ReadDir(dir)
	if len(des) != 1 || des[0].Name() != keyOf(tmplFor("keep")) {
		t.Fatalf("left on disk: %v", des)
	}
	// A removed template's URL no longer resolves (nothing on disk names it).
	u := "/art/" + keyOf(tmplFor("user")) + "/128x128.jpg"
	if tmpl, ok := c.Template(u); ok {
		t.Fatalf("removed artwork resolves to %q", tmpl)
	}
	// A restart indexes only what is left.
	if c2 := newCache(t, s, dir, 0); c2.Size() != c.Size() {
		t.Fatalf("reopened size %d, want %d", c2.Size(), c.Size())
	}
}

// Editorial playlist artwork uses a named crop instead of bb/cc
// ("…png/{w}x{h}SC.DN02.jpg"); only {w} and {h} are filled, the crop code
// is kept, and downloads carry a browser agent (Apple's CDN 403s a share
// of requests with Go's default one).
func TestEditorialCropTemplate(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Features221/v4/65/8a/52/x/4fff2b77.png/{w}x{h}SC.DN02.jpg"
	if u := c.URL(tmpl, 400, 400); !strings.HasSuffix(u, "/400x400.jpg") {
		t.Fatalf("URL = %q", u)
	}
	b, err := c.Get(t.Context(), tmpl, 400, 400)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 400 {
		t.Errorf("variant is %v", got)
	}
	want := "/image/thumb/Features221/v4/65/8a/52/x/4fff2b77.png/512x512SC.DN02.jpg"
	if n := s.count(host, want); n != 1 {
		t.Errorf("fetched %s %d times; hits %v", want, n, s.hits)
	}
	s.mu.Lock()
	ua := s.agents[want]
	s.mu.Unlock()
	if ua != DefaultUserAgent {
		t.Errorf("User-Agent = %q", ua)
	}
}

// A cover the CDN cannot serve yet (a new playlist's generated cover) is
// not cached as missing: the failure is returned, and the next request
// fetches again and succeeds.
func TestUnavailableCoverIsRetried(t *testing.T) {
	s := newImageServer(t)
	s.notReady = 1
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/gen/600x600AM.PDCXS11.jpg?c1=F2DE9D&t=U29uYQ"
	if _, err := c.Get(t.Context(), tmpl, 200, 200); err == nil {
		t.Fatal("a cover the CDN has not rendered yet was returned")
	}
	b, err := c.Get(t.Context(), tmpl, 200, 200)
	if err != nil || bounds(t, b).Dx() != 200 {
		t.Fatalf("retry: %v", err)
	}
}

// Wide editorial art ("{w}x{h}{c}.{f}", 2.4:1) is requested from Apple at
// the box's own width and height with {c} as "sr", never as a square, and
// requests of the same shape share one source.
func TestWideTemplateKeepsAspect(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	const dir = "/image/thumb/Features/v4/native-4320x1800/hero.png/"
	tmpl := "https://" + host + dir + "{w}x{h}{c}.{f}"
	u := c.URL(tmpl, 1000, 417)
	if !strings.HasSuffix(u, "/1000x417.jpg") {
		t.Fatalf("URL = %q", u)
	}
	b, err := c.Get(t.Context(), tmpl, 1000, 417)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 1000 || got.Dy() != 417 {
		t.Fatalf("variant is %v, want 1000x417", got)
	}
	if n := s.count(host, dir+"1024x427sr.jpg"); n != 1 || s.total() != 1 {
		t.Fatalf("want one 1024x427sr fetch; hits %v", s.hits)
	}
	// Another 2.4:1 box (rounded differently) reuses the source.
	b, err = c.Get(t.Context(), tmpl, 480, 200)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 480 || got.Dy() != 200 {
		t.Fatalf("480x200 variant is %v", got)
	}
	if s.total() != 1 {
		t.Fatalf("a 2.4:1 box refetched; hits %v", s.hits)
	}
	// A square box is another crop from Apple, not a cut of the wide source.
	if _, err := c.Get(t.Context(), tmpl, 400, 400); err != nil {
		t.Fatal(err)
	}
	if s.count(host, dir+"512x512sr.jpg") != 1 {
		t.Fatalf("square box: hits %v", s.hits)
	}
}

// A "bb" template fits the image inside the box, so a wide image asked for
// in a square box comes back wide, never stretched or padded.
func TestBoundingBoxTemplateFits(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Video/native-1920x1080/frame.jpg/{w}x{h}bb.jpg"
	b, err := c.Get(t.Context(), tmpl, 256, 256)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 256 || got.Dy() != 144 {
		t.Fatalf("variant is %v, want 256x144", got)
	}
}

// A "cc" template comes back as a w×w square whatever the box's height;
// the variant still fits its box.
func TestCentreCropTemplateFitsBox(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Features/native-4320x1080/chart.png/{w}x{h}cc.{f}"
	b, err := c.Get(t.Context(), tmpl, 400, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 100 || got.Dy() != 100 {
		t.Fatalf("variant is %v, want 100x100", got)
	}
}

func TestProportions(t *testing.T) {
	for _, tc := range []struct {
		tmpl         string
		w, h, ww, wh int
	}{
		{"https://a/{w}x{h}sr.jpg", 4320, 1800, 4320, 1800},
		{"https://a/{w}x{h}{c}.{f}", 1920, 1080, 1920, 1080},
		{"https://a/{w}x{h}cc.jpg", 4320, 1080, 1, 1},
		{"https://a/{w}x{h}bb.jpg", 0, 0, 1, 1},
	} {
		if w, h := Proportions(tc.tmpl, tc.w, tc.h); w != tc.ww || h != tc.wh {
			t.Errorf("Proportions(%q, %d, %d) = %d, %d", tc.tmpl, tc.w, tc.h, w, h)
		}
	}
}

// WithCrop swaps only the file name's crop code, whatever its spelling,
// keeping the extension and any "bb" elsewhere in the path.
func TestWithCrop(t *testing.T) {
	const dir = "https://is1-ssl.mzstatic.com/image/thumb/RadioSource221/v4/e2/bb_source.png/"
	for _, tc := range []struct{ tmpl, want string }{
		{dir + "{w}x{h}bb.{f}", dir + "{w}x{h}cc.{f}"},
		{dir + "{w}x{h}{c}.{f}", dir + "{w}x{h}cc.{f}"},
		{dir + "{w}x{h}SC.DN02.jpg", dir + "{w}x{h}cc.jpg"},
		{dir + "{w}x{h}cc.jpg", dir + "{w}x{h}cc.jpg"},
		{dir + "{w}x{h}.jpg", dir + "{w}x{h}cc.jpg"},
		// No {w}x{h} file name: nothing to crop.
		{dir + "600x600bb.jpg", dir + "600x600bb.jpg"},
		{"https://a/{w}x{h}/art", "https://a/{w}x{h}/art"},
		{"", ""},
	} {
		if got := WithCrop(tc.tmpl, "cc"); got != tc.want {
			t.Errorf("WithCrop(%q) = %q, want %q", tc.tmpl, got, tc.want)
		}
	}
}

// Boxes keep their aspect ratio while their long edge is clamped.
func TestURLClampsBox(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	tmpl := tmplFor("clamp")
	for _, tc := range []struct {
		w, h int
		want string
	}{
		{4096, 1024, "/2048x512.jpg"},
		{8, 2, "/16x4.jpg"},
		{0, 0, "/16x16.jpg"},
		{2048, 1, "/2048x1.jpg"},
	} {
		if u := c.URL(tmpl, tc.w, tc.h); !strings.HasSuffix(u, tc.want) {
			t.Errorf("URL(%d, %d) = %q, want …%s", tc.w, tc.h, u, tc.want)
		}
	}
}

// The square-only layout of earlier versions filled {c} with "bb", so its
// files are deleted at start rather than reused; the backdrop stays, and
// the old URLs refetch.
func TestLegacyFilesRemoved(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	tmpl := tmplFor("legacy")
	key := keyOf(tmpl)
	kdir := filepath.Join(dir, key)
	if err := os.MkdirAll(kdir, 0o755); err != nil {
		t.Fatal(err)
	}
	backdrop := DefaultBackdrop.name()
	for name, body := range map[string]string{
		"template": tmpl, "src-512.jpg": "old", "src-0.img": "old", "400.jpg": "old", backdrop: "keep",
	} {
		if err := os.WriteFile(filepath.Join(kdir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := newCache(t, s, dir, 0)
	des, _ := os.ReadDir(kdir)
	var left []string
	for _, de := range des {
		left = append(left, de.Name())
	}
	if strings.Join(left, " ") != backdrop+" template" {
		t.Fatalf("left on disk: %v", left)
	}
	if want := int64(len(tmpl) + len("keep")); c.Size() != want {
		t.Fatalf("size %d, want %d", c.Size(), want)
	}
	if b, err := c.Get(t.Context(), tmpl, 400, 400); err != nil || bounds(t, b).Dx() != 400 {
		t.Fatalf("variant after the legacy files went: %v", err)
	}
	if s.count(host, pathFor("legacy", 512)) != 1 {
		t.Fatalf("legacy source was not refetched; hits %v", s.hits)
	}
}

func TestTemplateOfServedPath(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	tmpl := tmplFor("t")
	u := c.URL(tmpl, 300, 300)
	for _, p := range []string{u, u + "?x=1", u + "#f", c.BackdropURL(tmpl)} {
		if got, ok := c.Template(p); !ok || got != tmpl {
			t.Errorf("Template(%q) = %q, %v", p, got, ok)
		}
	}
	// What it returns is what Get needs, at any box.
	b, err := c.Get(context.Background(), mustTemplate(t, c, u), 96, 96)
	if err != nil || bounds(t, b).Dx() != 96 {
		t.Fatalf("Get by template: %v, %v", bounds(t, b), err)
	}
	for _, p := range []string{
		"", "/art/", "/art/nokey/200x200.jpg", "/other/" + strings.Split(u, "/")[2] + "/1x1.jpg",
		"/art/" + strings.Repeat("0", 32) + "/200x200.jpg", "https://example.com/x.jpg",
	} {
		if got, ok := c.Template(p); ok {
			t.Errorf("Template(%q) = %q", p, got)
		}
	}
	// After a restart the key resolves from disk.
	c2 := newCache(t, s, dir, 0)
	if got, ok := c2.Template(u); !ok || got != tmpl {
		t.Errorf("after restart: %q, %v", got, ok)
	}
}

func mustTemplate(t *testing.T, c *Cache, u string) string {
	t.Helper()
	tmpl, ok := c.Template(u)
	if !ok {
		t.Fatalf("no template for %s", u)
	}
	return tmpl
}

func TestFetchesRunSideBySide(t *testing.T) {
	s := newImageServer(t)
	s.entered = make(chan struct{}, 32)
	s.gate = make(chan struct{})
	c := newCache(t, s, "", 0)
	const n = 12
	errs := make(chan error, n)
	for i := range n {
		go func() {
			_, err := c.Get(t.Context(), tmplFor("side"+string(rune('a'+i))), 64, 64)
			errs <- err
		}()
	}
	// Every download is at the server together, before any is answered: more
	// than the four the cache once allowed.
	for range n {
		within(t, "the downloads to arrive together", s.entered)
	}
	close(s.gate)
	for range n {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestAbandonedRequestStopsItsDownload(t *testing.T) {
	s := newImageServer(t)
	s.entered = make(chan struct{}, 4)
	s.gate = make(chan struct{})
	s.cancelled = make(chan struct{}, 4)
	c := newCache(t, s, "", 0)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := c.Get(ctx, tmplFor("left"), 64, 64); done <- err }()
	<-s.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Get after its context ended: %v", err)
	}
	within(t, "the server to see the download cancelled", s.cancelled)
	c.Wait()
	if sz := c.Size(); sz != 0 {
		t.Errorf("%d bytes cached from a download nobody waited for", sz)
	}
}

func TestAbandonedRequestKeepsTheDownloadAnotherCallerWaitsFor(t *testing.T) {
	s := newImageServer(t)
	s.entered = make(chan struct{}, 4)
	s.gate = make(chan struct{})
	s.cancelled = make(chan struct{}, 4)
	c := newCache(t, s, "", 0)
	ctx, cancel := context.WithCancel(t.Context())
	left := make(chan error, 1)
	go func() { _, err := c.Get(ctx, tmplFor("shared"), 64, 64); left <- err }()
	<-s.entered
	stays := make(chan error, 1)
	go func() { _, err := c.Get(t.Context(), tmplFor("shared"), 64, 64); stays <- err }()
	for !c.flight.waiting(2) { // both callers wait for the one download
		runtime.Gosched()
	}
	cancel()
	if err := <-left; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that left got %v", err)
	}
	close(s.gate)
	if err := <-stays; err != nil {
		t.Fatalf("the caller that stayed got %v", err)
	}
	select {
	case <-s.cancelled:
		t.Error("the download was cancelled while a caller waited for it")
	default:
	}
	if n := s.total(); n != 1 {
		t.Errorf("%d downloads, want the one both callers shared", n)
	}
}

func TestURLsMatchURL(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	longs := [3]int{96, 400, 1000}
	for _, tmpl := range []string{tmplFor("u"), "https://" + host + "/image/thumb/Features/hero.png/{w}x{h}{c}.{f}", "https://evil.example/{w}x{h}.jpg", ""} {
		for _, shape := range [][2]int{{1, 1}, {1000, 417}, {417, 1000}, {3000, 1}, {5, 3000}} {
			got := c.URLs(tmpl, shape[0], shape[1], longs)
			for i, long := range longs {
				l := max(shape[0], shape[1])
				want := c.URL(tmpl, (shape[0]*long+l/2)/l, (shape[1]*long+l/2)/l)
				if got[i] != want {
					t.Errorf("URLs(%q, %v)[%d] = %q, want %q", tmpl, shape, i, got[i], want)
				}
			}
		}
	}
}

// A removed template registers again, and its URL resolves again.
func TestURLRegistersAgainAfterRemove(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	tmpl := tmplFor("again")
	u := c.URL(tmpl, 128, 128)
	if _, err := c.Remove([]string{tmpl}); err != nil {
		t.Fatal(err)
	}
	if got, ok := c.Template(u); ok {
		t.Fatalf("removed template resolves to %q", got)
	}
	if c.URL(tmpl, 128, 128) != u {
		t.Fatal("URL changed")
	}
	if got, ok := c.Template(u); !ok || got != tmpl {
		t.Fatalf("Template after re-registering = %q, %v", got, ok)
	}
}

// BenchmarkURLs is the three art URLs of one song row: every track of an
// album shares a template, so the registry is hit after the first.
func BenchmarkURLs(b *testing.B) {
	c, err := New(Options{Dir: b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	tmpl := "https://" + host + "/image/thumb/Music/a/{w}x{h}bb.jpg"
	b.Run("three URL calls", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = c.URL(tmpl, 96, 96)
			_ = c.URL(tmpl, 400, 400)
			_ = c.URL(tmpl, 1000, 1000)
		}
	})
	b.Run("URLs", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = c.URLs(tmpl, 1, 1, [3]int{96, 400, 1000})
		}
	})
}
