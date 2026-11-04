package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Source serves one verified update to MyGo's updater (it satisfies
// mygo.UpdateSource). The manifest is the one already read; Open downloads
// the archive itself, checks its size and its Ed25519 signature against the
// build's key, and only then hands the bytes on. An archive that fails never
// reaches the installer, and MyGo checks the signature once more.
type Source struct {
	// HTTP is the download client (Sona's proxy-aware transport); nil means
	// a default client.
	HTTP *http.Client
	// Update is the manifest, Raw its bytes as published.
	Update Manifest
	Raw    []byte
	// Key is the base64 public key.
	Key string
	// Dir holds the download (next to the app so it can be renamed into
	// place; "" means the temp directory).
	Dir string
	// Progress reports bytes downloaded and the total, throttled.
	Progress func(done, total int64)
}

// Manifest returns the manifest read at check time.
func (s *Source) Manifest(context.Context, bool) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.Raw)), nil
}

// Open downloads and verifies the archive at url, which must be the
// manifest's.
func (s *Source) Open(ctx context.Context, url string) (io.ReadCloser, error) {
	m := s.Update
	if url != m.URL {
		return nil, fmt.Errorf("update: unexpected download %s", url)
	}
	pub, err := ParseKey(s.Key)
	if err != nil {
		return nil, err
	}
	if err := checkURL(url); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	hc := s.HTTP
	if hc == nil {
		hc = &http.Client{}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: downloading: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: downloading: %s", resp.Status)
	}
	f, err := os.CreateTemp(s.Dir, ".sona-update-*")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	sum := sha256.New()
	pr := &progressReader{r: io.LimitReader(resp.Body, m.Size+1), total: m.Size, fn: s.Progress}
	n, err := io.Copy(io.MultiWriter(f, sum), pr)
	if err != nil {
		return nil, fmt.Errorf("update: downloading: %w", err)
	}
	if n != m.Size {
		return nil, fmt.Errorf("update: downloaded %d bytes, want %d", n, m.Size)
	}
	if err := Verify(pub, sum.Sum(nil), m.Signature); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return &tempFile{f}, nil
}

// tempFile removes its file when closed.
type tempFile struct{ *os.File }

func (t *tempFile) Close() error {
	err := t.File.Close()
	_ = os.Remove(t.Name())
	return err
}

type progressReader struct {
	r     io.Reader
	n     int64
	total int64
	fn    func(done, total int64)
	last  time.Time
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if p.fn != nil && (err != nil || p.n == p.total || time.Since(p.last) > 100*time.Millisecond) {
		p.last = time.Now()
		p.fn(p.n, p.total)
	}
	return n, err
}
