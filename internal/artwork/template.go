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

// Sizes are the source variants fetched from Apple. A request for any size
// is served from the smallest cached source at least that large.
var Sizes = []int{64, 128, 256, 512, 1024, 2048}

const (
	// MinSize and MaxSize bound the edge length of a served variant.
	MinSize = 16
	MaxSize = 2048
)

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
	u, err := url.Parse(fill(tmpl, 1))
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

// fill substitutes Apple's placeholders: {w}/{h} with n, {c} (crop) with
// "bb" (fit in a bounding box) and {f} (format) with jpg.
func fill(tmpl string, n int) string {
	s := strconv.Itoa(n)
	return strings.NewReplacer("{w}", s, "{h}", s, "{c}", "bb", "{f}", "jpg").Replace(tmpl)
}

// standard returns the smallest standard source size covering size.
func standard(size int) int {
	if i := slices.IndexFunc(Sizes, func(s int) bool { return s >= size }); i >= 0 {
		return Sizes[i]
	}
	return Sizes[len(Sizes)-1]
}

func clampSize(size int) int { return min(max(size, MinSize), MaxSize) }
