package artwork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

// ServeHTTP serves "<prefix><key>/<w>x<h>.jpg" (and "<n>.jpg" for n×n,
// the square-only URLs of earlier versions) and "<prefix><key>/backdrop.jpg"
// for keys produced by URL and BackdropURL. Responses are immutable for a
// year and carry a strong ETag of the bytes; unknown keys, non-canonical
// names and boxes whose long edge is outside [MinSize, MaxSize] are 404s,
// so clients cannot make Sona fetch anything it did not hand out itself.
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, c.prefix)
	key, name, ok2 := strings.Cut(rest, "/")
	if !ok || !ok2 || !validKey(key) {
		http.NotFound(w, r)
		return
	}
	tmpl := c.lookup(key)
	if tmpl == "" {
		http.NotFound(w, r)
		return
	}
	var (
		path string
		err  error
	)
	if name == "backdrop.jpg" {
		path, err = c.backdropPath(r.Context(), key, tmpl)
	} else {
		stem, jpg := strings.CutSuffix(name, ".jpg")
		d, valid := parseDims(stem)
		if !jpg || !valid {
			http.NotFound(w, r)
			return
		}
		path, err = c.variant(r.Context(), key, tmpl, d)
	}
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			c.log.Warn("artwork: serve", "path", r.URL.Path, "err", err)
			// Often transient (a new playlist's cover is still rendering:
			// 406), so the webview must not keep the failure.
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "artwork unavailable", http.StatusBadGateway)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		w.Header().Set("Cache-Control", "no-store")
		http.Error(w, "artwork unavailable", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(data)
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}

// Template is the Apple artwork template behind a Go-served path
// ("/art/<key>/<w>x<h>.jpg", as URL and BackdropURL return, a query or
// fragment ignored), for a caller that wants the image without HTTP: it
// passes the template to Get at the exact size it needs. ok is false for a
// path that is not one this cache handed out.
func (c *Cache) Template(urlPath string) (template string, ok bool) {
	p, _, _ := strings.Cut(urlPath, "?")
	p, _, _ = strings.Cut(p, "#")
	rest, found := strings.CutPrefix(p, c.prefix)
	key, _, slash := strings.Cut(rest, "/")
	if !found || !slash || !validKey(key) {
		return "", false
	}
	template = c.lookup(key)
	return template, template != ""
}
