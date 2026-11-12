//go:build linux || (cgo && (darwin || windows))

package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/diag"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
	"github.com/coredev-uk/sona-go/internal/playback/soundcheck"
)

// Library uploads ("i." IDs without a catalog song) are plain audio files
// (MP3, AAC or ALAC), not HLS: webPlayback answers with one flavorless
// asset whose URL is the file. The engine downloads the file once into
// memory (it is not protected content) and streams the bytes to the host,
// which decodes them itself without the CDM. Seeks happen in the host,
// within the bytes it has, so the stream to it never restarts. Song
// previews (preview.go) take the same path.

// fileChunk is the most file bytes sent to the host in one request.
const fileChunk = 256 << 10

// upload is a plain audio file being downloaded: a library upload's, or a
// song's preview.
type upload struct {
	url    string
	cancel context.CancelFunc // ends the download

	mu   sync.Mutex
	wake chan struct{} // closed (and replaced) when data grows or the download ends
	data []byte        // the whole file; data[:n] is downloaded
	n    int
	size int64 // -1 until known
	done bool  // data[:n] is the whole file
	err  error // the download failed
}

func (u *upload) signal() {
	close(u.wake)
	u.wake = make(chan struct{})
}

// downloaded returns the part of the file downloaded so far.
func (u *upload) downloaded() []byte {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.data[:u.n]
}

// wait blocks until more than from bytes are downloaded or the download
// ended, and returns the downloaded part of the file.
func (u *upload) wait(ctx context.Context, from int) (data []byte, done bool, err error) {
	for {
		u.mu.Lock()
		data, done, err, w := u.data[:u.n], u.done, u.err, u.wake
		u.mu.Unlock()
		if len(data) > from || done || err != nil {
			return data, done, err
		}
		select {
		case <-w:
		case <-ctx.Done():
			return data, false, ctx.Err()
		}
	}
}

// sized blocks until the file's length is known.
func (u *upload) sized(ctx context.Context) (int64, error) {
	for {
		u.mu.Lock()
		size, err, w := u.size, u.err, u.wake
		u.mu.Unlock()
		if size >= 0 || err != nil {
			return size, err
		}
		select {
		case <-w:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// prepareUpload starts downloading the upload info describes and returns
// once its length is known.
func (e *Engine) prepareUpload(ctx context.Context, m *media, info applemusic.PlaybackInfo) error {
	if err := e.prepareFile(ctx, m, info.PlaylistURL); err != nil {
		return fmt.Errorf("native: upload %s: %w", m.id, err)
	}
	m.info = info
	return nil
}

// prepareFile starts downloading the plain audio file at url (an upload,
// or a song's preview) into m.file and returns once its length is known.
func (e *Engine) prepareFile(ctx context.Context, m *media, url string) error {
	dctx, cancel := context.WithCancel(e.bg)
	u := &upload{url: url, cancel: cancel, wake: make(chan struct{}), size: -1}
	diag.Go(e.log, "file download", func() { e.download(dctx, u) })
	size, err := u.sized(ctx)
	if err == nil {
		// Sound Check tags sit in the file's head (an MP4 movie box written
		// first, or an MP3's ID3v2 tag).
		var head []byte
		if head, _, err = u.wait(ctx, int(min(size, soundCheckHead))-1); err == nil {
			m.gain, _ = soundcheck.FromFile(head)
		}
	}
	if err != nil {
		cancel()
		return err
	}
	m.file = u
	e.log.Debug("prepared file", "id", m.id, "bytes", size, "gain", m.gain)
	return nil
}

// soundCheckHead is how much of a file is read for its Sound Check tag
// before it starts.
const soundCheckHead = 1 << 20

// download fetches u, resuming with a Range request after transient
// failures.
func (e *Engine) download(ctx context.Context, u *upload) {
	err := e.retrying(ctx, func() error { return e.downloadOnce(ctx, u) }, func(err error, delay time.Duration) time.Duration {
		e.log.Debug("retrying file download", "err", err, "in", delay)
		return delay
	})
	u.mu.Lock()
	if err != nil {
		u.err = err
	} else {
		u.done = true
	}
	u.signal()
	u.mu.Unlock()
}

func (e *Engine) downloadOnce(ctx context.Context, u *upload) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.url, nil)
	if err != nil {
		return err
	}
	u.mu.Lock()
	from := u.n
	u.mu.Unlock()
	if from > 0 {
		req.Header.Set("Range", "bytes="+strconv.Itoa(from)+"-")
	}
	req.Header.Set("User-Agent", applemusic.UserAgent)
	resp, err := e.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusPartialContent && from > 0:
	case resp.StatusCode == http.StatusOK:
		if from > 0 { // Range ignored: skip what we have
			if _, err := io.CopyN(io.Discard, resp.Body, int64(from)); err != nil {
				return err
			}
		}
	default:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return &httpError{url: "file", code: resp.StatusCode}
	}
	u.mu.Lock()
	if u.size < 0 {
		if resp.StatusCode != http.StatusOK || resp.ContentLength < 0 {
			u.mu.Unlock()
			return e.downloadUnsized(u, resp.Body)
		}
		u.size, u.data = resp.ContentLength, make([]byte, resp.ContentLength)
		u.signal()
	}
	data := u.data
	u.mu.Unlock()
	for from < len(data) {
		// Writes go past n, which readers never look beyond.
		k, err := resp.Body.Read(data[from:])
		if k > 0 {
			from += k
			u.mu.Lock()
			u.n = from
			u.signal()
			u.mu.Unlock()
		}
		if errors.Is(err, io.EOF) {
			if from < len(data) {
				return fmt.Errorf("GET file: %w after %d of %d bytes", io.ErrUnexpectedEOF, from, len(data))
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("GET file: %w", err)
		}
	}
	return nil
}

// downloadUnsized reads a response without a length to the end; the file
// becomes available all at once.
func (e *Engine) downloadUnsized(u *upload, body io.Reader) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("GET file: %w", err)
	}
	u.mu.Lock()
	u.data, u.n, u.size = b, len(b), int64(len(b))
	u.signal()
	u.mu.Unlock()
	return nil
}

// load hands m to the host as track tok, starting at pos (a live
// broadcast joins where it is).
func (e *Engine) load(ctx context.Context, h host, tok uint64, m *media, pos time.Duration) error {
	if m.broadcast != nil {
		return h.LoadFile(ctx, tok, gst.Live, 0, 0)
	}
	if m.file != nil {
		return h.LoadFile(ctx, tok, m.file.size, pos, m.gain)
	}
	if err := h.Load(ctx, tok, m.hostMedia(m.init), pos); err != nil {
		return err
	}
	return e.loadVideo(ctx, h, tok, m, pos)
}

// feedChunks feeds b to the host as fileChunk-sized pieces from file
// offset off, and returns the offset reached (short of the end on error).
func feedChunks(ctx context.Context, h host, tok uint64, off int64, b []byte) (int64, error) {
	for len(b) > 0 {
		n := min(len(b), fileChunk)
		if err := h.FeedFile(ctx, tok, off, b[:n]); err != nil {
			return off, err
		}
		off += int64(n)
		b = b[n:]
	}
	return off, nil
}

// feedFile streams the file m to the host as it downloads, for the file
// loaded as track tok. Seeks keep it running.
func (e *Engine) feedFile(ctx context.Context, it *item, tok uint64, h host, m *media) {
	for off := int64(0); ; {
		data, done, err := m.file.wait(ctx, int(off))
		if err != nil {
			e.feedFailed(ctx, it, fmt.Errorf("native: file %s: %w", m.id, err))
			return
		}
		if off, err = feedChunks(ctx, h, tok, off, data[off:]); err != nil {
			e.feedFailed(ctx, it, err)
			return
		}
		if done {
			if err := h.EndOfFile(ctx, tok); err != nil {
				e.feedFailed(ctx, it, err)
			}
			return
		}
	}
}
