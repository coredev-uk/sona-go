package video

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

func TestSurfaceNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewSurface()
		ctx := t.Context()

		// Nothing on show: Next waits until ctx ends.
		short, cancel := context.WithTimeout(ctx, time.Second)
		f, ok := s.Next(short, 0)
		cancel()
		if ok || f.Seq != 0 {
			t.Fatalf("empty surface: %+v %v", f, ok)
		}

		data := []byte{1, 2, 3, 4, 5, 6}
		s.Show(2, 2, 40*time.Millisecond, data)
		f, ok = s.Next(ctx, 0)
		if !ok || f.Seq != 1 || f.Width != 2 || f.Height != 2 || f.Position != 40*time.Millisecond || !bytes.Equal(f.Data, data) {
			t.Fatalf("first picture: %+v %v", f, ok)
		}

		// Next blocks until a picture newer than after is shown.
		got := make(chan Frame, 1)
		go func() {
			f, _ := s.Next(ctx, 1)
			got <- f
		}()
		synctest.Wait()
		select {
		case f := <-got:
			t.Fatalf("Next returned %+v before a newer picture", f)
		default:
		}
		s.Show(2, 2, 80*time.Millisecond, data)
		if f := <-got; f.Seq != 2 || f.Position != 80*time.Millisecond {
			t.Fatalf("second picture: %+v", f)
		}

		// Clear wakes a waiter with false and the new sequence.
		type result struct {
			f  Frame
			ok bool
		}
		cleared := make(chan result, 1)
		go func() {
			f, ok := s.Next(ctx, 2)
			cleared <- result{f, ok}
		}()
		synctest.Wait()
		s.Clear()
		if r := <-cleared; r.ok || r.f.Seq != 3 {
			t.Fatalf("after Clear: %+v %v", r.f, r.ok)
		}
		// A second Clear changes nothing.
		s.Clear()
		if f, ok := s.Next(ctx, 0); ok || f.Seq != 3 {
			t.Fatalf("cleared surface: %+v %v", f, ok)
		}
		// A seq at the current one waits.
		short, cancel = context.WithTimeout(ctx, time.Second)
		defer cancel()
		if f, ok := s.Next(short, 3); ok || f.Seq != 0 {
			t.Fatalf("Next(3) on a cleared surface: %+v %v", f, ok)
		}
	})
}

func serve(s *Surface, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, target, nil))
	return rec
}

func TestSurfaceServeHTTP(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewSurface()
		data := make([]byte, 3*3+2*2*2) // I420 3×3
		for i := range data {
			data[i] = byte(i)
		}

		// No picture within the poll: 204, no sequence.
		start := time.Now()
		rec := serve(s, http.MethodGet, Path)
		if rec.Code != http.StatusNoContent || rec.Header().Get("X-Video-Seq") != "" || rec.Body.Len() != 0 {
			t.Fatalf("empty: %d %q %x", rec.Code, rec.Header().Get("X-Video-Seq"), rec.Body.Bytes())
		}
		if d := time.Since(start); d != poll {
			t.Fatalf("answered after %v, want %v", d, poll)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("Cache-Control %q", got)
		}

		s.Show(3, 3, 1234*time.Millisecond+500*time.Microsecond, data)
		rec = serve(s, http.MethodGet, Path+"?after=0")
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		h := rec.Header()
		if h.Get("Content-Type") != "application/octet-stream" || h.Get("X-Video-Seq") != "1" || h.Get("Cache-Control") != "no-store" {
			t.Fatalf("headers %v", h)
		}
		body := rec.Body.Bytes()
		want := []byte{'S', 'V', 'F', '1', 1, 0, 0, 0, 3, 0, 3, 0}
		want = binary.LittleEndian.AppendUint32(want, 1234)
		want = append(want, data...)
		if !bytes.Equal(body, want) {
			t.Fatalf("body %x\nwant %x", body, want)
		}
		if h.Get("Content-Length") != "33" {
			t.Fatalf("Content-Length %q", h.Get("Content-Length"))
		}

		// The client has picture 1: it waits for the next one.
		got := make(chan *httptest.ResponseRecorder, 1)
		go func() { got <- serve(s, http.MethodGet, Path+"?after=1") }()
		synctest.Wait()
		s.Show(864, 486, 0, data)
		rec = <-got
		if rec.Code != http.StatusOK || rec.Header().Get("X-Video-Seq") != "2" {
			t.Fatalf("second: %d %v", rec.Code, rec.Header())
		}
		if hdr := rec.Body.Bytes()[:headerSize]; binary.LittleEndian.Uint16(hdr[8:]) != 864 || binary.LittleEndian.Uint16(hdr[10:]) != 486 ||
			binary.LittleEndian.Uint32(hdr[4:]) != 2 {
			t.Fatalf("header %x", hdr)
		}

		// Nothing newer than 2 within the poll.
		if rec := serve(s, http.MethodGet, Path+"?after=2"); rec.Code != http.StatusNoContent || rec.Header().Get("X-Video-Seq") != "" {
			t.Fatalf("timeout: %d %v", rec.Code, rec.Header())
		}

		// Cleared: 204 with the sequence to wait from.
		go func() { got <- serve(s, http.MethodGet, Path+"?after=2") }()
		synctest.Wait()
		s.Clear()
		rec = <-got
		if rec.Code != http.StatusNoContent || rec.Header().Get("X-Video-Seq") != "3" || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("cleared: %d %v", rec.Code, rec.Header())
		}

		if rec := serve(s, http.MethodPost, Path); rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST: %d", rec.Code)
		}
	})
}

func TestEncodeHeader(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    Frame
		want []byte
	}{
		{
			name: "plain", f: Frame{Seq: 7, Width: 864, Height: 486, Position: 90 * time.Second},
			want: []byte{'S', 'V', 'F', '1', 7, 0, 0, 0, 0x60, 0x03, 0xe6, 0x01, 0x90, 0x5f, 0x01, 0x00},
		},
		{
			name: "sequence wraps", f: Frame{Seq: 1<<32 + 5, Width: 2, Height: 2},
			want: []byte{'S', 'V', 'F', '1', 5, 0, 0, 0, 2, 0, 2, 0, 0, 0, 0, 0},
		},
		{
			name: "negative position", f: Frame{Seq: 1, Width: 2, Height: 2, Position: -time.Second},
			want: []byte{'S', 'V', 'F', '1', 1, 0, 0, 0, 2, 0, 2, 0, 0, 0, 0, 0},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, headerSize)
			encodeHeader(b, tc.f)
			if !bytes.Equal(b, tc.want) {
				t.Fatalf("header %x, want %x", b, tc.want)
			}
		})
	}
}

func TestSurfaceSeq(t *testing.T) {
	s := NewSurface()
	if got := s.Seq(); got != 0 {
		t.Fatalf("empty surface Seq %d, want 0", got)
	}
	s.Show(2, 2, 0, make([]byte, 6))
	if got := s.Seq(); got != 1 {
		t.Errorf("after a picture Seq %d, want 1", got)
	}
	s.Clear()
	if got := s.Seq(); got != 2 {
		t.Errorf("after the clear Seq %d, want 2", got)
	}
	s.Clear() // nothing on show: no change
	if got := s.Seq(); got != 2 {
		t.Errorf("a clear of nothing moved Seq to %d", got)
	}
}
