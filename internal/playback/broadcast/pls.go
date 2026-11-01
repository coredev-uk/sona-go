package broadcast

import (
	"bufio"
	"bytes"
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// isPLS reports whether a playlist is a PLS file ("[playlist]").
func isPLS(b []byte) bool {
	return bytes.HasPrefix(bytes.ToLower(bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\ufeff")))), []byte("[playlist]"))
}

// parsePLS returns the stream URLs of a PLS playlist in entry order
// (File1, File2, …).
func parsePLS(b []byte) []string {
	type entry struct {
		n   int
		url string
	}
	var entries []entry
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if !ok || len(k) < 5 || !strings.EqualFold(k[:4], "file") {
			continue
		}
		n, err := strconv.Atoi(k[4:])
		if v = strings.TrimSpace(v); err == nil && v != "" {
			entries = append(entries, entry{n, v})
		}
	}
	slices.SortStableFunc(entries, func(a, b entry) int { return cmp.Compare(a.n, b.n) })
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.url
	}
	return out
}

// isHLS reports whether an M3U playlist is an HLS playlist (it has
// EXT-X tags) rather than a plain list of stream URLs.
func isHLS(b []byte) bool { return bytes.Contains(b, []byte("#EXT-X-")) }

// parseM3U returns the URLs of a plain M3U playlist in order.
func parseM3U(b []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if l := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff")); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out
}
