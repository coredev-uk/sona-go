// Package motion is Sona's motion artwork cache. Apple's motion artwork
// (editorialVideo) is an unencrypted HLS master playlist on its media CDN
// whose renditions are fragmented MP4: one file read by byte ranges
// (albums), or an initialisation file and segment files (editorial
// playlists). The cache picks the rendition for the size the UI draws (in
// the codec the web view plays), fetches its files once, remuxes them into
// a plain MP4 whose timeline starts at 0 (WebKitGTK does not play Apple's
// fragments, see flatten), and serves it to the web view (with Range
// support) at /motion/<key>/<size>.mp4, so the UI never contacts
// Apple and a looping video plays from disk. Least-recently-used files are
// evicted over a size cap.
package motion

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	defaultMaxBytes = 256 << 20
	maxPlaylist     = 1 << 20
	maxFile         = 64 << 20
	fetchTimeout    = 2 * time.Minute
	// touchEvery bounds how often a hit refreshes a file's mtime, which
	// orders eviction.
	touchEvery = time.Hour
	// DefaultSize is the size in a URL handed out by URL; the UI
	// replaces it with the size it draws.
	DefaultSize = 720
)

// sizes are the rendition buckets a requested size falls into (device
// pixels), each the first of Apple's H.264 square ladder (360, 486, 768,
// 960 and 1080 wide) that is at least that wide: cover tiles fit the first,
// header covers the second, album covers the third, the now-playing cover
// (up to 400 DIPs, 800 px at 2x) the fourth, so no cover is drawn from a
// narrower picture.
var sizes = []int{360, 480, 720, 960}

// bucket maps a requested size to its rendition bucket: the smallest at
// least that large, else the largest.
func bucket(px int) int {
	for _, s := range sizes {
		if px <= s {
			return s
		}
	}
	return sizes[len(sizes)-1]
}

// userAgent is sent to Apple's media CDN, as the artwork cache does.
const userAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36"

// Options configures New.
type Options struct {
	Dir string // cache directory, created if missing ($XDG_CACHE_HOME/sona/motion)
	// HTTP fetches from Apple's public media CDN; no Apple credentials are
	// sent. nil uses http.DefaultClient.
	HTTP     *http.Client
	Logger   *slog.Logger // nil discards
	MaxBytes int64        // disk cap; 0 means 256 MiB
	Prefix   string       // URL path the handler is mounted at; "" means "/motion/"
}

// Cache hands out motion artwork URLs and serves them.
type Cache struct {
	dir    string
	http   *http.Client
	log    *slog.Logger
	max    int64
	prefix string
	// allow vets every Apple URL before it is fetched.
	allow func(*url.URL) bool

	mu      sync.Mutex
	masters map[string]string // key → HLS master playlist URL
	touched map[string]time.Time
	last    Fetch
	flight  singleflight.Group
	evictMu sync.Mutex
}

// ErrNoRendition is a master playlist with no rendition in the codec asked
// for.
var ErrNoRendition = errors.New("motion: no rendition in the codec")

// Fetch is the outcome of the newest rendition download, for Diagnostics.
type Fetch struct {
	At    time.Time
	Key   string
	Size  int    // rendition bucket (device pixels)
	Codec string // CodecH264 or CodecHEVC
	// Rendition is the variant chosen ("avc1.64001f 486×486, 1.7
	// Mbit/s"), "" when none was.
	Rendition string
	Err       error // nil when it worked
}

// Stats describes the cache for Diagnostics.
type Stats struct {
	Files int   // renditions on disk
	Bytes int64 // their size
	Last  Fetch // the newest download since start (zero At: none)
}

// New opens the cache at o.Dir.
func New(o Options) (*Cache, error) {
	if err := os.MkdirAll(o.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("motion cache: %w", err)
	}
	c := &Cache{
		dir:     o.Dir,
		log:     cmp.Or(o.Logger, slog.New(slog.DiscardHandler)),
		max:     cmp.Or(o.MaxBytes, defaultMaxBytes),
		prefix:  cmp.Or(o.Prefix, "/motion/"),
		allow:   appleMedia,
		masters: map[string]string{},
		touched: map[string]time.Time{},
	}
	// Every redirect hop is vetted like the first URL: an Apple host's 3xx
	// must not send the fetch to another host, scheme or port.
	client := *cmp.Or(o.HTTP, http.DefaultClient)
	next := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !c.allow(req.URL) {
			return fmt.Errorf("motion: refusing redirect to %s", req.URL.Redacted())
		}
		if next != nil {
			return next(req, via)
		}
		if len(via) >= 5 {
			return errors.New("motion: too many redirects")
		}
		return nil
	}
	c.http = &client
	return c, nil
}

// appleMedia accepts https URLs on Apple's media and image CDNs, on the
// default port and without userinfo.
func appleMedia(u *url.URL) bool {
	h := u.Hostname()
	return u.Scheme == "https" && u.User == nil && (u.Port() == "" || u.Port() == "443") &&
		(strings.HasSuffix(h, ".apple.com") || strings.HasSuffix(h, ".mzstatic.com"))
}

// URL returns the Go-served URL of the motion artwork whose HLS master
// playlist is master (applemusic.EditorialVideo.Square), at DefaultSize;
// "" for an empty or foreign URL.
func (c *Cache) URL(master string) string {
	if c == nil || master == "" {
		return ""
	}
	u, err := url.Parse(master)
	if err != nil || !c.allow(u) {
		return ""
	}
	sum := sha256.Sum256([]byte(master))
	key := hex.EncodeToString(sum[:12])
	c.mu.Lock()
	c.masters[key] = master
	c.mu.Unlock()
	return c.prefix + key + "/" + strconv.Itoa(DefaultSize) + ".mp4"
}

func validKey(k string) bool {
	if len(k) != 24 {
		return false
	}
	_, err := hex.DecodeString(k)
	return err == nil
}

// ServeHTTP serves "<prefix><key>/<size>.mp4" for keys handed out by URL
// (or already on disk), sizes 16–2048, in H.264, or in HEVC with
// "?codec=hevc" (for a web view that plays HEVC but not H.264). The first
// request for a rendition waits for its download; later ones, and Range
// requests while it loops, read the file on disk. Unknown keys are 404s,
// so the web view cannot make Sona fetch anything it did not hand out
// itself.
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, c.prefix)
	key, name, ok2 := strings.Cut(rest, "/")
	px, err := strconv.Atoi(strings.TrimSuffix(name, ".mp4"))
	codec := cmp.Or(r.URL.Query().Get("codec"), CodecH264)
	if !ok || !ok2 || !validKey(key) || err != nil || strconv.Itoa(px)+".mp4" != name || px < 16 || px > 2048 ||
		(codec != CodecH264 && codec != CodecHEVC) {
		http.NotFound(w, r)
		return
	}
	path, err := c.ensure(r.Context(), key, bucket(px), codec)
	switch {
	case errors.Is(err, ErrUnknown):
		http.NotFound(w, r)
		return
	case r.Context().Err() != nil:
		return
	case err != nil:
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "motion artwork unavailable: "+err.Error(), http.StatusBadGateway)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "motion artwork unavailable", http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	c.touch(path)
	h := w.Header()
	h.Set("Content-Type", "video/mp4")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, f)
}

// ErrUnknown is a motion artwork this cache did not hand out (a key URL
// never returned, and no file of it on disk).
var ErrUnknown = errors.New("motion: unknown artwork")

// Local returns the file of the motion artwork urlPath names
// ("<prefix><key>/<n>.mp4" as URL returns it, a query ignored) at px device
// pixels (16–2048) in codec (CodecH264 when ""), downloading it first when
// it is not cached: the file ServeHTTP serves, for a caller in this process
// (the native UI's decoder) that reads it without HTTP. The download is
// shared with ServeHTTP's and completes even if ctx ends. The file may be
// evicted later; ask again rather than keeping paths. A path this cache did
// not hand out is ErrUnknown.
func (c *Cache) Local(ctx context.Context, urlPath string, px int, codec string) (string, error) {
	p, _, _ := strings.Cut(urlPath, "?")
	rest, ok := strings.CutPrefix(p, c.prefix)
	key, _, ok2 := strings.Cut(rest, "/")
	codec = cmp.Or(codec, CodecH264)
	if !ok || !ok2 || !validKey(key) || px < 16 || px > 2048 || (codec != CodecH264 && codec != CodecHEVC) {
		return "", ErrUnknown
	}
	return c.ensure(ctx, key, bucket(px), codec)
}

// ensure returns the file of key's rendition at the bucket size in codec,
// downloading it when it is not on disk. ErrUnknown when key was never
// handed out and has no file.
func (c *Cache) ensure(ctx context.Context, key string, size int, codec string) (string, error) {
	path := c.path(key, size, codec)
	if _, err := os.Stat(path); err == nil {
		c.touch(path) // a file in use stays out of the way of eviction
		return path, nil
	}
	c.mu.Lock()
	master := c.masters[key]
	c.mu.Unlock()
	if master == "" {
		return "", ErrUnknown
	}
	// Detached: the download completes for the next view even if this one
	// navigates away.
	ch := c.flight.DoChan(filepath.Base(path), func() (any, error) {
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		start := time.Now()
		rendition, err := c.fetch(fctx, master, size, codec, path)
		f := Fetch{At: time.Now(), Key: key, Size: size, Codec: codec, Rendition: rendition}
		if err != nil {
			f.Err = err
			c.log.Warn("motion: fetch", "key", key, "size", size, "codec", codec, "rendition", rendition, "err", err)
		} else {
			c.log.Info("motion: fetched", "key", key, "size", size, "rendition", rendition, "took_ms", time.Since(start).Milliseconds())
		}
		c.mu.Lock()
		c.last = f
		c.mu.Unlock()
		return nil, err
	})
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return "", res.Err
		}
	}
	c.touch(path)
	return path, nil
}

// Stats counts the renditions on disk and reports the newest download.
func (c *Cache) Stats() Stats {
	var s Stats
	if entries, err := os.ReadDir(c.dir); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".mp4") || strings.HasPrefix(e.Name(), tempPrefix) {
				continue
			}
			if info, err := e.Info(); err == nil {
				s.Files++
				s.Bytes += info.Size()
			}
		}
	}
	c.mu.Lock()
	s.Last = c.last
	c.mu.Unlock()
	return s
}

// path is a rendition's file: "<key>-<size>.mp4" in H.264 (as before
// HEVC could be asked for), "<key>-<size>-hevc.mp4" in HEVC.
func (c *Cache) path(key string, size int, codec string) string {
	name := key + "-" + strconv.Itoa(size)
	if codec != CodecH264 {
		name += "-" + codec
	}
	return filepath.Join(c.dir, name+".mp4")
}

// touch refreshes path's mtime (its place in eviction order) at most once
// per touchEvery.
func (c *Cache) touch(path string) {
	now := time.Now()
	c.mu.Lock()
	last := c.touched[path]
	if now.Sub(last) < touchEvery {
		c.mu.Unlock()
		return
	}
	c.touched[path] = now
	c.mu.Unlock()
	_ = os.Chtimes(path, now, now)
}

// fetch resolves master to the rendition of codec for size and stores its
// file at path. It returns the rendition chosen ("" before one is).
func (c *Cache) fetch(ctx context.Context, master string, size int, codec, path string) (string, error) {
	mu, err := url.Parse(master)
	if err != nil {
		return "", err
	}
	body, err := c.get(ctx, mu, maxPlaylist)
	if err != nil {
		return "", fmt.Errorf("master playlist: %w", err)
	}
	v, ok := choose(parseMaster(string(body)), size, codec)
	if !ok {
		return "", fmt.Errorf("%w: no %s rendition", ErrNoRendition, codecName(codec))
	}
	rendition := v.describe()
	vu, err := mu.Parse(v.URI)
	if err != nil {
		return rendition, err
	}
	body, err = c.get(ctx, vu, maxPlaylist)
	if err != nil {
		return rendition, fmt.Errorf("media playlist: %w", err)
	}
	files, err := parseMedia(string(body), vu)
	if err != nil {
		return rendition, err
	}
	parts := make([][]byte, 0, len(files))
	var total int64
	for _, fu := range files {
		b, err := c.get(ctx, fu, maxFile-total)
		if err != nil {
			return rendition, fmt.Errorf("video: %w", err)
		}
		total += int64(len(b))
		parts = append(parts, b)
	}
	flat, err := flatten(parts...)
	if err != nil {
		return rendition, fmt.Errorf("video %s: %w", files[len(files)-1].Path, err)
	}
	tmp, err := os.CreateTemp(c.dir, tempPrefix+"*")
	if err != nil {
		return rendition, err
	}
	_, err = tmp.Write(flat)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
		return rendition, fmt.Errorf("video: %w", err)
	}
	c.evict(path)
	return rendition, nil
}

func codecName(codec string) string {
	if codec == CodecHEVC {
		return "HEVC"
	}
	return "H.264"
}

// tempPrefix names downloads in progress.
const tempPrefix = ".download-"

// get fetches u (an allowed Apple URL) whole, up to limit bytes.
func (c *Cache) get(ctx context.Context, u *url.URL, limit int64) ([]byte, error) {
	var b bytes.Buffer
	if err := c.copy(ctx, u, &b, limit); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// copy writes u (an allowed Apple URL) to dst, failing past limit bytes.
func (c *Cache) copy(ctx context.Context, u *url.URL, dst io.Writer, limit int64) error {
	if !c.allow(u) {
		return fmt.Errorf("refusing %s", u.Redacted())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", u.Path, resp.StatusCode)
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("%s: larger than %d bytes", u.Path, limit)
	}
	return nil
}

// evict removes downloads abandoned by a crash, then the least recently
// used files until the cache fits its cap. It never removes keep, the file
// just downloaded to answer a request: files written within the same clock
// tick have equal times, and the unstable sort could otherwise pick it.
func (c *Cache) evict(keep string) {
	c.evictMu.Lock()
	defer c.evictMu.Unlock()
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type file struct {
		path string
		size int64
		mod  time.Time
	}
	var files []file
	var total int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(c.dir, e.Name())
		switch {
		case strings.HasPrefix(e.Name(), tempPrefix):
			if time.Since(info.ModTime()) > fetchTimeout {
				_ = os.Remove(path)
			}
		case strings.HasSuffix(e.Name(), ".mp4"):
			total += info.Size()
			if path != keep {
				files = append(files, file{path, info.Size(), info.ModTime()})
			}
		}
	}
	slices.SortFunc(files, func(a, b file) int { return a.mod.Compare(b.mod) })
	for _, f := range files {
		if total <= c.max {
			break
		}
		if os.Remove(f.path) == nil {
			total -= f.size
		}
	}
}
