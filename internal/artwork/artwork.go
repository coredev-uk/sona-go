// Package artwork is Sona's artwork disk cache. It turns Apple artwork
// templates ({w}x{h} URLs on *.mzstatic.com) into Go-served URLs for a
// width×height box, fetches each source size and aspect ratio once (Apple
// fills or crops the box by the template's crop code), derives exact-size
// JPEG variants, pre-blurred backdrops and dominant colours from disk, and
// evicts least-recently-used artwork when the cache exceeds its size cap.
// Sources must be JPEG or PNG, recognised by
// their magic bytes and decoded by image/jpeg or image/png directly; anything
// else is refused with ErrFormat before any decoder runs.
package artwork

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxBytes = 512 << 20
	// defaultFetches is how many sources download at once: Apple's CDN
	// answers over HTTP/2, which carries them as streams of a few
	// connections.
	defaultFetches = 16
	maxSourceBytes = 16 << 20
	fetchTimeout   = 30 * time.Second
	variantQuality = 88
	templateFile   = "template"
	// touchEvery bounds how often a hit refreshes a key directory's mtime,
	// which is what orders eviction after a restart.
	touchEvery = time.Hour
)

// DefaultUserAgent is sent with artwork downloads. Apple's image CDN
// answers a share of requests carrying Go's default "Go-http-client" agent
// with an empty 403 (observed 2026-10-30: 25–90 of 300 fresh sizes, none
// with any other agent), so a browser agent like the rest of Sona's Apple
// traffic is used.
const DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"

// Options configures New.
type Options struct {
	Dir string // cache root, created if missing ($XDG_CACHE_HOME/sona/art)
	// HTTP fetches from the public artwork CDN; no Apple credentials are
	// sent, so this is a plain client, not applemusic.Client. nil uses
	// http.DefaultClient.
	HTTP     *http.Client
	Logger   *slog.Logger // nil discards
	MaxBytes int64        // disk cap; 0 means 512 MiB
	Prefix   string       // URL path the Handler is mounted at; "" means "/art/"
	// CoverDir holds the covers the user chose from files (SaveCover); it
	// is user data, not cache, so it sits outside Dir. "" stores none.
	CoverDir string
	Fetches  int // concurrent downloads; 0 means 16
	// UserAgent overrides DefaultUserAgent.
	UserAgent string
	Backdrop  BackdropStyle
}

// BackdropStyle shapes Backdrop output. A zero Size selects DefaultBackdrop.
type BackdropStyle struct {
	Size       int     // output edge in px
	Sigma      float64 // Gaussian blur sigma in output px
	Brightness float64 // percent, -100..100 (negative darkens)
	Saturation float64 // percent, -100..500 (positive saturates)
	Quality    int     // JPEG quality
}

// DefaultBackdrop is a 64 px, strongly blurred, slightly darkened and
// saturated JPEG of a few kilobytes, meant to be stretched by CSS.
var DefaultBackdrop = BackdropStyle{Size: 64, Sigma: 4, Brightness: -15, Saturation: 25, Quality: 80}

func (s BackdropStyle) name() string {
	return fmt.Sprintf("backdrop-%d-%g-%g-%g-%d.jpg", s.Size, s.Sigma, s.Brightness, s.Saturation, s.Quality)
}

// Cache is the artwork disk cache. It is safe for concurrent use.
type Cache struct {
	dir      string
	http     *http.Client
	ua       string
	log      *slog.Logger
	max      int64
	prefix   string
	covers   string
	backdrop BackdropStyle
	fetchSem chan struct{} // bounds downloads
	workSem  chan struct{} // bounds decode/resize/encode
	flight   flights

	mu        sync.Mutex
	templates map[string]string // key → template, registered by URL and Get
	keys      map[string]*entry // on-disk index
	total     int64
	colors    map[string][]Color
}

// entry indexes one key directory.
type entry struct {
	files   map[string]int64 // name → bytes
	used    time.Time        // last access, for eviction
	touched time.Time        // last directory mtime refresh
	busy    int              // in-flight operations; never evicted while > 0
}

// New opens (and indexes) the cache at opts.Dir.
func New(opts Options) (*Cache, error) {
	if opts.Dir == "" {
		return nil, errors.New("artwork: Dir is required")
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("artwork: create cache dir: %w", err)
	}
	hc := http.DefaultClient
	if opts.HTTP != nil {
		hc = opts.HTTP
	}
	client := *hc
	next := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := checkURL(req.URL); err != nil {
			return err
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= 5 {
			return errors.New("artwork: too many redirects")
		}
		return nil
	}
	c := &Cache{
		dir:       opts.Dir,
		http:      &client,
		ua:        cmp.Or(opts.UserAgent, DefaultUserAgent),
		log:       opts.Logger,
		max:       opts.MaxBytes,
		prefix:    opts.Prefix,
		covers:    opts.CoverDir,
		backdrop:  opts.Backdrop,
		fetchSem:  make(chan struct{}, cmp.Or(max(opts.Fetches, 0), defaultFetches)),
		workSem:   make(chan struct{}, runtime.GOMAXPROCS(0)),
		templates: map[string]string{},
		keys:      map[string]*entry{},
		colors:    map[string][]Color{},
	}
	if c.log == nil {
		c.log = slog.New(slog.DiscardHandler)
	}
	if c.max <= 0 {
		c.max = defaultMaxBytes
	}
	if c.prefix == "" {
		c.prefix = "/art/"
	}
	if !strings.HasSuffix(c.prefix, "/") {
		c.prefix += "/"
	}
	if c.backdrop.Size == 0 {
		c.backdrop = DefaultBackdrop
	}
	if err := c.scan(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.evictLocked()
	c.mu.Unlock()
	return c, nil
}

// scan builds the index from disk and removes interrupted writes and the
// files of the square-only layout (legacyName).
func (c *Cache) scan() error {
	des, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("artwork: read cache dir: %w", err)
	}
	for _, de := range des {
		if !de.IsDir() || !validKey(de.Name()) {
			continue
		}
		kdir := filepath.Join(c.dir, de.Name())
		files, err := os.ReadDir(kdir)
		if err != nil {
			continue
		}
		e := &entry{files: map[string]int64{}}
		if info, err := de.Info(); err == nil {
			e.used, e.touched = info.ModTime(), info.ModTime()
		}
		for _, f := range files {
			if strings.HasPrefix(f.Name(), ".tmp-") || legacyName(f.Name()) {
				_ = os.Remove(filepath.Join(kdir, f.Name()))
				continue
			}
			if info, err := f.Info(); err == nil && info.Mode().IsRegular() {
				e.files[f.Name()] = info.Size()
				c.total += info.Size()
			}
		}
		c.keys[de.Name()] = e
	}
	return nil
}

// Size reports the bytes currently on disk.
func (c *Cache) Size() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

// MaxBytes reports the disk cap.
func (c *Cache) MaxBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.max
}

// SetMaxBytes changes the disk cap (n <= 0 means 512 MiB) and evicts least
// recently used artwork at once if the cache is over it.
func (c *Cache) SetMaxBytes(n int64) {
	if n <= 0 {
		n = defaultMaxBytes
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.max = n
	c.evictLocked()
}

// Remove deletes the cached artwork of refs, each an artwork template or a
// URL from URL or BackdropURL, and forgets their templates, so their URLs
// stop resolving until the templates are registered again. Invalid refs are
// skipped. It returns the bytes freed.
func (c *Cache) Remove(refs []string) (int64, error) {
	keys := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if rest, ok := strings.CutPrefix(ref, c.prefix); ok {
			if key, _, _ := strings.Cut(rest, "/"); validKey(key) {
				keys[key] = true
			}
		} else if _, key, err := parseTemplate(ref); err == nil {
			keys[key] = true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var freed int64
	var errs []error
	for key := range keys {
		delete(c.templates, key)
		delete(c.colors, key)
		if err := os.RemoveAll(filepath.Join(c.dir, key)); err != nil {
			errs = append(errs, fmt.Errorf("artwork: remove %s: %w", key, err))
			continue
		}
		e := c.keys[key]
		if e == nil {
			continue
		}
		for _, n := range e.files {
			freed += n
		}
		if e.busy > 0 {
			e.files = map[string]int64{}
		} else {
			delete(c.keys, key)
		}
	}
	c.total -= freed
	return freed, errors.Join(errs...)
}

// URL returns the Go-served path of template's variant for a w×h box
// ("/art/<key>/<w>x<h>.jpg"), or "" when template is empty or not an Apple
// artwork URL. The box keeps its aspect ratio with its long edge clamped to
// [MinSize, MaxSize]. Nothing is fetched until the URL is requested from
// Handler.
func (c *Cache) URL(template string, w, h int) string {
	key, err := c.register(template)
	if err != nil {
		return ""
	}
	return c.prefix + key + "/" + clampDims(w, h).String() + ".jpg"
}

// BackdropURL returns the Go-served path of template's backdrop
// ("/art/<key>/backdrop.jpg"), or "" for an invalid template.
func (c *Cache) BackdropURL(template string) string {
	key, err := c.register(template)
	if err != nil {
		return ""
	}
	return c.prefix + key + "/backdrop.jpg"
}

// Get returns template's variant for a w×h box (clamped as in URL) as JPEG
// bytes, fetching a source from Apple at most once per standard size and
// aspect ratio.
func (c *Cache) Get(ctx context.Context, template string, w, h int) ([]byte, error) {
	p, err := c.LocalPath(ctx, template, w, h)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// LocalPath is Get returning the JPEG's path on disk, e.g. for MPRIS
// mpris:artUrl ("file://" + path). The file may be evicted later; call again
// rather than holding paths across tracks.
func (c *Cache) LocalPath(ctx context.Context, template string, w, h int) (string, error) {
	key, err := c.register(template)
	if err != nil {
		return "", err
	}
	return c.variant(ctx, key, c.templateOf(key), clampDims(w, h))
}

// Backdrop returns template's pre-blurred backdrop JPEG (see BackdropStyle).
func (c *Cache) Backdrop(ctx context.Context, template string) ([]byte, error) {
	key, err := c.register(template)
	if err != nil {
		return nil, err
	}
	p, err := c.backdropPath(ctx, key, c.templateOf(key))
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// Clear deletes every cached file. Registered URLs stay valid and refetch
// on demand.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	des, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("artwork: clear: %w", err)
	}
	var errs []error
	for _, de := range des {
		if validKey(de.Name()) {
			errs = append(errs, os.RemoveAll(filepath.Join(c.dir, de.Name())))
		}
	}
	for k, e := range c.keys {
		if e.busy > 0 {
			e.files = map[string]int64{}
		} else {
			delete(c.keys, k)
		}
	}
	c.total = 0
	clear(c.colors)
	return errors.Join(errs...)
}

// register validates template and remembers it under its key.
func (c *Cache) register(template string) (string, error) {
	tmpl, key, err := parseTemplate(template)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.templates[key] = tmpl
	c.mu.Unlock()
	return key, nil
}

func (c *Cache) templateOf(key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.templates[key]
}

// lookup resolves a key from the registry or, after a restart, from the
// template file stored with the artwork. Unknown keys resolve to "".
func (c *Cache) lookup(key string) string {
	if t := c.templateOf(key); t != "" {
		return t
	}
	b, err := os.ReadFile(filepath.Join(c.dir, key, templateFile))
	if err != nil {
		return ""
	}
	tmpl, k, err := parseTemplate(string(b))
	if err != nil || k != key {
		return ""
	}
	c.mu.Lock()
	c.templates[key] = tmpl
	c.mu.Unlock()
	return tmpl
}

// acquire pins key against eviction until release.
func (c *Cache) acquire(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.keys[key]
	if e == nil {
		e = &entry{files: map[string]int64{}}
		c.keys[key] = e
	}
	e.busy++
}

func (c *Cache) release(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.keys[key]
	e.busy--
	if e.busy == 0 && len(e.files) == 0 {
		delete(c.keys, key)
	}
}

// hit returns the path of key/name if indexed and marks the key used.
func (c *Cache) hit(key, name string) (string, bool) {
	c.mu.Lock()
	e := c.keys[key]
	if e == nil {
		c.mu.Unlock()
		return "", false
	}
	if _, ok := e.files[name]; !ok {
		c.mu.Unlock()
		return "", false
	}
	now := time.Now()
	e.used = now
	touch := now.Sub(e.touched) > touchEvery
	if touch {
		e.touched = now
	}
	c.mu.Unlock()
	dir := filepath.Join(c.dir, key)
	if touch {
		_ = os.Chtimes(dir, now, now)
	}
	return filepath.Join(dir, name), true
}

// store atomically writes key/name and accounts for it, evicting other keys
// if the cache is over its cap.
func (c *Cache) store(key, tmpl, name string, data []byte) (string, error) {
	dir := filepath.Join(c.dir, key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("artwork: %w", err)
	}
	c.mu.Lock()
	_, haveTmpl := c.keys[key].files[templateFile]
	c.mu.Unlock()
	if !haveTmpl && tmpl != "" {
		if err := c.writeFile(key, templateFile, []byte(tmpl)); err != nil {
			return "", err
		}
	}
	if err := c.writeFile(key, name, data); err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func (c *Cache) writeFile(key, name string, data []byte) error {
	dir := filepath.Join(c.dir, key)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("artwork: %w", err)
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(f.Name(), filepath.Join(dir, name))
	}
	if werr != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("artwork: write %s: %w", name, werr)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.keys[key]
	now := time.Now()
	c.total += int64(len(data)) - e.files[name]
	e.files[name] = int64(len(data))
	e.used, e.touched = now, now
	c.evictLocked()
	return nil
}

// evictLocked removes least-recently-used idle keys until the cache is at
// 90% of its cap.
func (c *Cache) evictLocked() {
	if c.total <= c.max {
		return
	}
	type cand struct {
		key  string
		used time.Time
	}
	var cands []cand
	for k, e := range c.keys {
		if e.busy == 0 {
			cands = append(cands, cand{k, e.used})
		}
	}
	slices.SortFunc(cands, func(a, b cand) int { return a.used.Compare(b.used) })
	low := c.max / 10 * 9
	for _, cd := range cands {
		if c.total <= low {
			break
		}
		if err := os.RemoveAll(filepath.Join(c.dir, cd.key)); err != nil {
			c.log.Warn("artwork: evict", "key", cd.key, "err", err)
			continue
		}
		for _, n := range c.keys[cd.key].files {
			c.total -= n
		}
		delete(c.keys, cd.key)
		delete(c.colors, cd.key)
	}
}

// sourceName names a source fetched for box d: "src-<w>x<h>.jpg" for a JPEG
// within its box, which a request for exactly that box is served as-is,
// and ".img" for anything else (a PNG, Apple's w×w square for a "cc" crop,
// or a fixed template's single source, "src-0x0").
func sourceName(d dims, data []byte, cfg image.Config) string {
	if isJPEG(data) && cfg.Width <= d.w && cfg.Height <= d.h {
		return "src-" + d.String() + ".jpg"
	}
	return "src-" + d.String() + ".img"
}

// sourceDimsOf parses the box of a source file name.
func sourceDimsOf(name string) (dims, bool) {
	rest, ok := strings.CutPrefix(name, "src-")
	if !ok {
		return dims{}, false
	}
	rest, _, _ = strings.Cut(rest, ".")
	ws, hs, ok := strings.Cut(rest, "x")
	w, err1 := strconv.Atoi(ws)
	h, err2 := strconv.Atoi(hs)
	return dims{w, h}, ok && err1 == nil && err2 == nil
}

// cachedSource finds the smallest indexed source covering want (or the
// single source of a fixed-size template).
func (c *Cache) cachedSource(key string, want dims, fixedSize bool) (path string, src dims, ok bool) {
	c.mu.Lock()
	e := c.keys[key]
	bestName := ""
	if e != nil {
		for name := range e.files {
			d, found := sourceDimsOf(name)
			if !found || (fixedSize && d != dims{}) || (!fixedSize && !d.covers(want)) {
				continue
			}
			if bestName == "" || d.w*d.h < src.w*src.h {
				src, bestName = d, name
			}
		}
	}
	c.mu.Unlock()
	if bestName == "" {
		return "", dims{}, false
	}
	p, ok := c.hit(key, bestName)
	return p, src, ok
}

// source returns a source image covering want, fetching it if needed. A
// synthetic template's source (a mosaic composed from its tiles' sources, a
// stored cover) is always square: a request for another shape is served by
// fitting that square.
func (c *Cache) source(ctx context.Context, key, tmpl string, want dims) (string, dims, error) {
	synth := synthetic(tmpl)
	if synth {
		want = dims{want.long(), want.long()}
	}
	fx := !synth && fixed(tmpl)
	sd := sourceDims(want)
	if fx {
		sd = dims{}
	}
	if p, d, ok := c.cachedSource(key, want, fx); ok {
		return p, d, nil
	}
	type src struct {
		path string
		d    dims
	}
	v, err := c.flight.do(ctx, "src/"+key+"/"+sd.String(), func(fctx context.Context) (any, error) {
		c.acquire(key)
		defer c.release(key)
		if p, d, ok := c.cachedSource(key, want, fx); ok {
			return src{p, d}, nil
		}
		var (
			data []byte
			cfg  image.Config
			err  error
		)
		if synth {
			data, err = c.synthesise(fctx, tmpl, sd.w)
			cfg = image.Config{Width: sd.w, Height: sd.h}
		} else {
			u := tmpl
			if !fx {
				u = fill(tmpl, sd)
			}
			data, cfg, err = c.fetch(fctx, u)
		}
		if err != nil {
			return nil, err
		}
		p, err := c.store(key, tmpl, sourceName(sd, data, cfg), data)
		return src{p, sd}, err
	})
	if err != nil {
		return "", dims{}, err
	}
	s := v.(src)
	return s.path, s.d, nil
}

// synthesise makes the side×side JPEG of a synthetic template.
func (c *Cache) synthesise(ctx context.Context, tmpl string, side int) ([]byte, error) {
	if isMosaic(tmpl) {
		return c.compose(ctx, tmpl, side)
	}
	return c.renderCover(ctx, tmpl, side)
}

// fetch downloads one artwork URL and reads its image header. ctx is the
// flight's, which ends when nobody waits for the source any more.
func (c *Cache) fetch(ctx context.Context, u string) ([]byte, image.Config, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	select {
	case c.fetchSem <- struct{}{}:
	case <-ctx.Done():
		return nil, image.Config{}, fmt.Errorf("artwork: fetch: %w", ctx.Err())
	}
	defer func() { <-c.fetchSem }()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("artwork: %w", err)
	}
	if err := checkURL(req.URL); err != nil {
		return nil, image.Config{}, err
	}
	// Apple's generated artwork (a playlist's gradient or pattern with its
	// name, gen/…AM.PDCXS1n.jpg) is drawn on a CDN miss by an origin that
	// answers 406 to an Accept without */* (checked 2026-11-06: image/jpeg,
	// image/png and image/* alone are refused, */* gets image/jpeg). A size
	// already in the CDN's cache is served whatever Accept says, so the
	// failure only shows at sizes Apple's own apps never ask for.
	req.Header.Set("Accept", "image/jpeg,image/png;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", c.ua)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("artwork: fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, image.Config{}, fmt.Errorf("artwork: fetch %s: %s", req.URL.Redacted(), resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceBytes+1))
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("artwork: fetch: %w", err)
	}
	if len(data) > maxSourceBytes {
		return nil, image.Config{}, errors.New("artwork: source too large")
	}
	cfg, err := decodeConfig(data)
	if err != nil {
		return nil, image.Config{}, fmt.Errorf("artwork: fetch %s: %w", req.URL.Redacted(), err)
	}
	c.log.Debug("artwork: fetched", "url", req.URL.Redacted(), "bytes", len(data))
	return data, cfg, nil
}

// variant returns the path of key's JPEG for box d, deriving it if needed.
func (c *Cache) variant(ctx context.Context, key, tmpl string, d dims) (string, error) {
	if tmpl == "" {
		return "", errors.New("artwork: unknown key")
	}
	c.acquire(key)
	defer c.release(key)
	name := d.String() + ".jpg"
	if p, ok := c.hit(key, name); ok {
		return p, nil
	}
	if p, ok := c.hit(key, "src-"+name); ok {
		return p, nil
	}
	src, sd, err := c.source(ctx, key, tmpl, d)
	if err != nil {
		return "", err
	}
	if sd == d && strings.HasSuffix(src, ".jpg") {
		return src, nil
	}
	return c.derive(ctx, key, tmpl, name, func() ([]byte, error) {
		img, err := c.decode(src)
		if err != nil {
			return nil, err
		}
		img = fit(img, d.w, d.h)
		return encodeJPEG(img, variantQuality)
	})
}

// backdropPath returns the path of key's backdrop, rendering it if needed.
func (c *Cache) backdropPath(ctx context.Context, key, tmpl string) (string, error) {
	if tmpl == "" {
		return "", errors.New("artwork: unknown key")
	}
	c.acquire(key)
	defer c.release(key)
	st := c.backdrop
	name := st.name()
	if p, ok := c.hit(key, name); ok {
		return p, nil
	}
	src, _, err := c.source(ctx, key, tmpl, dims{st.Size, st.Size})
	if err != nil {
		return "", err
	}
	return c.derive(ctx, key, tmpl, name, func() ([]byte, error) {
		img, err := c.decode(src)
		if err != nil {
			return nil, err
		}
		out := cover(img, st.Size)
		blur(out, st.Sigma)
		adjust(out, st.Saturation, st.Brightness)
		return encodeJPEG(out, st.Quality)
	})
}

// Wait blocks until the flights callers gave up on (a cancelled context)
// have returned. Tests call it before removing the cache's directory; nothing
// may ask for artwork meanwhile.
func (c *Cache) Wait() { c.flight.wait() }

// derive renders key/name once across concurrent callers.
func (c *Cache) derive(ctx context.Context, key, tmpl, name string, render func() ([]byte, error)) (string, error) {
	v, err := c.flight.do(ctx, "out/"+key+"/"+name, func(fctx context.Context) (any, error) {
		c.acquire(key)
		defer c.release(key)
		if p, ok := c.hit(key, name); ok {
			return p, nil
		}
		select {
		case c.workSem <- struct{}{}:
		case <-fctx.Done():
			return "", fctx.Err()
		}
		data, err := render()
		<-c.workSem
		if err != nil {
			return "", err
		}
		return c.store(key, tmpl, name, data)
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// decode decodes a cached source, which fetch has already checked is a JPEG
// or PNG.
func (c *Cache) decode(path string) (image.Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("artwork: %w", err)
	}
	img, err := decodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("artwork: decode %s: %w", filepath.Base(path), err)
	}
	return img, nil
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("artwork: encode: %w", err)
	}
	return buf.Bytes(), nil
}
