package artwork

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Sizes are the long edges of the sources fetched from Apple. A request
// is served from the smallest cached source at least that large with the
// same aspect ratio.
var Sizes = []int{64, 128, 256, 512, 1024, 2048}

const (
	// MinSize and MaxSize bound the long edge of a served variant; its
	// short edge is at least 1.
	MinSize = 16
	MaxSize = 2048
)

// dims is an artwork box in pixels: Apple fills a template's {w} and {h}
// with it, and a variant fits inside it.
type dims struct{ w, h int }

// String is the box as it appears in file and URL names: "<w>x<h>".
func (d dims) String() string { return strconv.Itoa(d.w) + "x" + strconv.Itoa(d.h) }

func (d dims) long() int { return max(d.w, d.h) }

// scaled returns d with its long edge set to long, keeping its aspect
// ratio (each edge at least 1).
func (d dims) scaled(long int) dims {
	l := d.long()
	return dims{max(1, (d.w*long+l/2)/l), max(1, (d.h*long+l/2)/l)}
}

// covers reports whether a source fetched for box d can serve a request
// for r: at least as large (1 px of rounding slack) and of the same aspect
// ratio within 2%, because Apple crops to the box for most crop codes.
func (d dims) covers(r dims) bool {
	if d.w+1 < r.w || d.h+1 < r.h {
		return false
	}
	diff := d.w*r.h - d.h*r.w
	return 50*max(diff, -diff) <= d.h*r.w
}

// clampDims keeps w×h's aspect ratio while bringing its long edge into
// [MinSize, MaxSize]; each edge is at least 1.
func clampDims(w, h int) dims {
	d := dims{max(w, 1), max(h, 1)}
	if l := d.long(); l > MaxSize || l < MinSize {
		return d.scaled(min(max(l, MinSize), MaxSize))
	}
	return d
}

// parseDims parses a variant name without ".jpg": "<w>x<h>", or a bare
// "<n>" for n×n (the square-only URLs of earlier versions, still held in
// recent searches). Only the canonical spelling of an in-range box is
// accepted, so one box has one file.
func parseDims(s string) (dims, bool) {
	ws, hs, wide := strings.Cut(s, "x")
	if !wide {
		hs = ws
	}
	w, err1 := strconv.Atoi(ws)
	h, err2 := strconv.Atoi(hs)
	d := dims{w, h}
	if err1 != nil || err2 != nil || clampDims(w, h) != d {
		return dims{}, false
	}
	if wide {
		return d, d.String() == s
	}
	return d, strconv.Itoa(w) == s
}

// ErrHost reports a template outside Apple's artwork CDN.
var ErrHost = errors.New("artwork: host not allowed")

// allowedHost reports whether host serves Apple artwork. Only Apple's
// artwork CDN (*.mzstatic.com, e.g. is1-ssl.mzstatic.com) is accepted, so
// keys and redirects can never make Sona fetch an arbitrary host.
func allowedHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".mzstatic.com") && len(host) > len(".mzstatic.com")
}

// checkURL validates a template or filled artwork URL.
func checkURL(u *url.URL) error {
	if u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("%w: %s", ErrHost, u.Redacted())
	}
	if p := u.Port(); p != "" && p != "443" {
		return fmt.Errorf("%w: %s", ErrHost, u.Host)
	}
	if !allowedHost(u.Hostname()) {
		return fmt.Errorf("%w: %s", ErrHost, u.Hostname())
	}
	return nil
}

// parseTemplate normalises and validates an Apple artwork template such as
// https://is1-ssl.mzstatic.com/image/thumb/…/{w}x{h}bb.jpg and returns it
// with its cache key.
func parseTemplate(tmpl string) (string, string, error) {
	tmpl = strings.TrimSpace(tmpl)
	if tmpl == "" {
		return "", "", errors.New("artwork: empty template")
	}
	u, err := url.Parse(fill(tmpl, dims{1, 1}))
	if err != nil {
		return "", "", fmt.Errorf("artwork: parse template: %w", err)
	}
	if err := checkURL(u); err != nil {
		return "", "", err
	}
	return tmpl, keyOf(tmpl), nil
}

// keyOf is the content-addressed cache key of a normalised template.
func keyOf(tmpl string) string {
	sum := sha256.Sum256([]byte(tmpl))
	return hex.EncodeToString(sum[:16])
}

// validKey reports whether s looks like a key produced by keyOf.
func validKey(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// fixed reports whether tmpl has no size placeholder (a plain image URL).
func fixed(tmpl string) bool { return !strings.Contains(tmpl, "{w}") }

// fill substitutes Apple's placeholders: {w} and {h} with d, {c} (crop)
// with "sr" and {f} (format) with jpg. Apple's crop codes decide what a
// box means (checked against the CDN, 2026-11-01): "bb" fits the image
// inside it; "sr" (and named crops such as "SC.DN02") crop it to the box,
// which at the image's own aspect ratio is the whole image, the same bytes
// as "bb"; "cc" crops a w×w square whatever h is; "mv" sizes by w alone.
func fill(tmpl string, d dims) string {
	return strings.NewReplacer("{w}", strconv.Itoa(d.w), "{h}", strconv.Itoa(d.h), "{c}", "sr", "{f}", "jpg").Replace(tmpl)
}

// Proportions returns the width and height a template's artwork is shown
// at, from the width and height Apple lists for it: a square for "cc"
// templates (Apple crops those to w×w) and for unknown sizes.
func Proportions(tmpl string, width, height int) (int, int) {
	if width <= 0 || height <= 0 || strings.Contains(tmpl, "{h}cc.") {
		return 1, 1
	}
	return width, height
}

// WithCrop returns tmpl with its crop code (what follows "{w}x{h}" in the
// file name up to the extension: "bb", "sr", "{c}", "SC.DN02") replaced
// by code, e.g. "cc" for the centre square Apple's apps cut from wide key
// art. A template without a "{w}x{h}" file name is returned unchanged.
func WithCrop(tmpl, code string) string {
	const box = "{w}x{h}"
	i := strings.LastIndex(tmpl, box) + len(box)
	if i < len(box) {
		return tmpl
	}
	ext := strings.LastIndexByte(tmpl, '.')
	if ext < i || strings.ContainsRune(tmpl[i:], '/') {
		return tmpl
	}
	return tmpl[:i] + code + tmpl[ext:]
}

// sourceDims is the box a source is fetched at for a request: the request's
// aspect ratio with its long edge raised to the smallest standard size
// covering it, so requests of one shape share at most len(Sizes) sources.
func sourceDims(d dims) dims {
	l := d.long()
	if i := slices.IndexFunc(Sizes, func(s int) bool { return s >= l }); i >= 0 {
		return d.scaled(Sizes[i])
	}
	return d.scaled(Sizes[len(Sizes)-1])
}

// legacyName reports a file of the square-only cache layout ("<n>.jpg"
// variants, "src-<n>.jpg|.img" sources). Those filled {c} with "bb", so
// they are deleted rather than reused; they refetch on demand.
func legacyName(name string) bool {
	stem := strings.TrimPrefix(name, "src-")
	stem, _, _ = strings.Cut(stem, ".")
	_, err := strconv.Atoi(stem)
	return err == nil && (strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".img"))
}
