package cdm

import (
	"bytes"
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// stallTimeout ends a download that receives nothing for this long.
const stallTimeout = 60 * time.Second

// download fetches rel's package into a temporary file in dir, trying
// its URLs in order, and returns the file once its size and SHA-512 are
// the manifest's. Each URL, and every redirect, must be on hosts.
func download(ctx context.Context, hc *http.Client, hosts []string, rel Release, dir string, progress func(received, total int64)) (string, error) {
	c := restricted(hc, hosts)
	var errs []error
	for _, u := range rel.URLs {
		p, err := downloadFrom(ctx, c, u, rel, dir, progress)
		if err == nil {
			return p, nil
		}
		errs = append(errs, err)
		if ctx.Err() != nil {
			break
		}
	}
	return "", firstCoded(errs)
}

func downloadFrom(ctx context.Context, c *http.Client, u string, rel Release, dir string, progress func(received, total int64)) (_ string, err error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fail(CodeNetwork, "GET %s: HTTP %d", u, resp.StatusCode)
	}
	if resp.ContentLength > rel.Size {
		return "", fail(CodeTooLarge, "GET %s: %d bytes, the manifest says %d", u, resp.ContentLength, rel.Size)
	}
	f, err := os.CreateTemp(dir, partPrefix+"*.crx3")
	if err != nil {
		return "", &Error{Code: CodeDisk, Err: err}
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = &Error{Code: CodeDisk, Err: cerr}
		}
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()

	// A stalled transfer is cancelled; every read rearms the timer.
	stall := time.AfterFunc(stallTimeout, func() { cancel(fmt.Errorf("no data for %s", stallTimeout)) })
	defer stall.Stop()
	h := sha512.New()
	var n int64
	buf := make([]byte, 256<<10)
	body := io.LimitReader(resp.Body, rel.Size+1)
	for {
		m, rerr := body.Read(buf)
		if m > 0 {
			stall.Reset(stallTimeout)
			if _, werr := f.Write(buf[:m]); werr != nil {
				return "", &Error{Code: CodeDisk, Err: werr}
			}
			h.Write(buf[:m])
			n += int64(m)
			if n > rel.Size {
				return "", fail(CodeTooLarge, "GET %s: more than the manifest's %d bytes", u, rel.Size)
			}
			if progress != nil {
				progress(n, rel.Size)
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			if cause := context.Cause(ctx); cause != nil && ctx.Err() != nil {
				rerr = cause
			}
			return "", fail(CodeNetwork, "GET %s: %w", u, rerr)
		}
	}
	if n != rel.Size {
		return "", fail(CodeHashMismatch, "GET %s: %d bytes, the manifest says %d", u, n, rel.Size)
	}
	if sum := h.Sum(nil); !bytes.Equal(sum, rel.SHA512) {
		return "", fail(CodeHashMismatch, "GET %s: SHA-512 %x, the manifest says %x", u, sum, rel.SHA512)
	}
	if err := f.Sync(); err != nil {
		return "", &Error{Code: CodeDisk, Err: err}
	}
	return f.Name(), nil
}
