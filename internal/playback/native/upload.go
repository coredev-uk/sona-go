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
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/soundcheck"
)

// Library uploads ("i." IDs without a catalog song) are plain audio files
// (MP3, AAC or ALAC), not HLS: webPlayback answers with one flavorless
// asset whose URL is the file. The engine downloads the file once into
// memory (it is not protected content) and streams the bytes to the host,
// which decodes them itself without the CDM. Seeks happen in the host,
// within the bytes it has, so the stream to it never restarts.

// fileChunk is the most file bytes sent to the host in one request.
const fileChunk = 256 << 10

// upload is a library upload's file being downloaded.
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
	dctx, cancel := context.WithCancel(e.bg)
	u := &upload{url: info.PlaylistURL, cancel: cancel, wake: make(chan struct{}), size: -1}
	diag.Go(e.log, "upload download", func() { e.download(dctx, u) })
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
		return fmt.Errorf("native: upload %s: %w", m.id, err)
	}
	m.info, m.file = info, u
	e.log.Debug("prepared upload", "id", m.id, "bytes", size, "gain", m.gain)
	return nil
}

// soundCheckHead is how much of an upload is read for its Sound Check tag
// before it starts.
const soundCheckHead = 1 << 20

// download fetches u, resuming with a Range request after transient
// failures.
func (e *Engine) download(ctx context.Context, u *upload) {
	var err error
	for attempt := 0; ; attempt++ {
		if err = e.downloadOnce(ctx, u); err == nil || attempt >= len(e.retry) || !retryable(err) || ctx.Err() != nil {
			break
		}
		e.log.Debug("retrying upload download", "err", err, "in", e.retry[attempt])
		select {
		case <-time.After(e.retry[attempt]):
		case <-ctx.Done():
		}
	}
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
		return &httpError{url: "upload", code: resp.StatusCode}
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
				return fmt.Errorf("GET upload: %w after %d of %d bytes", io.ErrUnexpectedEOF, from, len(data))
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("GET upload: %w", err)
		}
	}
	return nil
}

// downloadUnsized reads a response without a length to the end; the file
// becomes available all at once.
func (e *Engine) downloadUnsized(u *upload, body io.Reader) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("GET upload: %w", err)
	}
	u.mu.Lock()
	u.data, u.n, u.size = b, len(b), int64(len(b))
	u.signal()
	u.mu.Unlock()
	return nil
}

// load hands m to the host as track tok, starting at pos.
func (e *Engine) load(ctx context.Context, h host, tok uint64, m *media, pos time.Duration) error {
	if m.file != nil {
		return h.LoadFile(ctx, tok, m.file.size, pos, m.gain)
	}
	return h.Load(ctx, tok, m.hostMedia(m.init), pos)
}

// feedFile streams the upload m to the host as it downloads, for the file
// loaded as track tok. Seeks keep it running.
func (e *Engine) feedFile(ctx context.Context, it *item, tok uint64, h host, m *media) {
	failed := func(err error) {
		if ctx.Err() == nil && !errors.Is(err, cdmhost.ErrStale) && !errors.Is(err, cdmhost.ErrExited) {
			_ = e.fail(it, err)
		}
	}
	for off := 0; ; {
		data, done, err := m.file.wait(ctx, off)
		if err != nil {
			failed(fmt.Errorf("native: upload %s: %w", m.id, err))
			return
		}
		for off < len(data) {
			end := min(off+fileChunk, len(data))
			if err := h.FeedFile(ctx, tok, int64(off), data[off:end]); err != nil {
				failed(err)
				return
			}
			off = end
		}
		if done {
			if err := h.EndOfFile(ctx, tok); err != nil {
				failed(err)
			}
			return
		}
	}
}
