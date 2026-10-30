// Package motion is Sona's motion artwork cache. Apple's motion artwork
// (editorialVideo) is an unencrypted HLS master playlist on its media CDN
// whose H.264 renditions are each one fragmented MP4 file read by byte
// ranges. The cache picks the rendition for the size the UI draws, fetches
// that file once, remuxes it into a plain MP4 whose timeline starts at 0
// (WebKitGTK does not play Apple's fragments, see flatten), and serves it
// to the web view (with Range support) at /motion/<key>/<size>.mp4, so the
// frontend never contacts Apple and a looping video plays from disk.
// Least-recently-used files are evicted over a size cap.
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
	// DefaultSize is the size in a URL handed out by URL; the frontend
	// replaces it with the size it draws.
	DefaultSize = 720
)

// sizes are the rendition buckets a requested size falls into (device
// pixels): header covers fit the first, the now-playing cover the second.
var sizes = []int{480, 720}

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
	flight  singleflight.Group
	evictMu sync.Mutex
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
// (or already on disk), sizes 16–2048. The first request for a rendition
// waits for its download; later ones, and Range requests while it loops,
// read the file on disk. Unknown keys are 404s, so the web view cannot
// make Sona fetch anything it did not hand out itself.
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, c.prefix)
	key, name, ok2 := strings.Cut(rest, "/")
	px, err := strconv.Atoi(strings.TrimSuffix(name, ".mp4"))
	if !ok || !ok2 || !validKey(key) || err != nil || strconv.Itoa(px)+".mp4" != name || px < 16 || px > 2048 {
		http.NotFound(w, r)
		return
	}
	size := bucket(px)
	path := c.path(key, size)
	if _, err := os.Stat(path); err != nil {
		c.mu.Lock()
		master := c.masters[key]
		c.mu.Unlock()
		if master == "" {
			http.NotFound(w, r)
			return
		}
		// Detached: the download completes for the next view even if
		// this one navigates away.
		ch := c.flight.DoChan(filepath.Base(path), func() (any, error) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), fetchTimeout)
			defer cancel()
			return nil, c.fetch(ctx, master, size, path)
		})
		select {
		case <-r.Context().Done():
			return
		case res := <-ch:
			if res.Err != nil {
				c.log.Warn("motion: fetch", "key", key, "size", size, "err", res.Err)
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "motion artwork unavailable", http.StatusBadGateway)
				return
			}
		}
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

func (c *Cache) path(key string, size int) string {
	return filepath.Join(c.dir, key+"-"+strconv.Itoa(size)+".mp4")
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

// fetch resolves master to the rendition for size and stores its file at
// path.
func (c *Cache) fetch(ctx context.Context, master string, size int, path string) error {
	mu, err := url.Parse(master)
	if err != nil {
		return err
	}
	body, err := c.get(ctx, mu, maxPlaylist)
	if err != nil {
		return fmt.Errorf("master playlist: %w", err)
	}
	v, ok := choose(parseMaster(string(body)), size)
	if !ok {
		return errors.New("no H.264 rendition")
	}
	vu, err := mu.Parse(v.URI)
	if err != nil {
		return err
	}
	body, err = c.get(ctx, vu, maxPlaylist)
	if err != nil {
		return fmt.Errorf("media playlist: %w", err)
	}
	fu, err := parseMedia(string(body), vu)
	if err != nil {
		return err
	}
	file, err := c.get(ctx, fu, maxFile)
	if err != nil {
		return fmt.Errorf("video: %w", err)
	}
	flat, err := flatten(file)
	if err != nil {
		return fmt.Errorf("video %s: %w", fu.Path, err)
	}
	tmp, err := os.CreateTemp(c.dir, tempPrefix+"*")
	if err != nil {
		return err
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
		return fmt.Errorf("video: %w", err)
	}
	c.evict()
	return nil
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
// used files until the cache fits its cap.
func (c *Cache) evict() {
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
			files = append(files, file{path, info.Size(), info.ModTime()})
			total += info.Size()
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
