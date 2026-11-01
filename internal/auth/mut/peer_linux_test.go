package mut

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestOwnerListenerDropsOtherUsers checks the real /proc lookup: a listener
// expecting another uid drops this process's connections unanswered, one
// expecting ours serves them.
func TestOwnerListenerDropsOtherUsers(t *testing.T) {
	serve := func(uid uint32) string {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ol := ownerOnly(ln).(*ownerListener)
		ol.uid = uid
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }), ReadHeaderTimeout: time.Second}
		go func() { _ = srv.Serve(ol) }()
		t.Cleanup(func() { _ = srv.Close() })
		return ln.Addr().String()
	}
	get := func(addr string) (string, error) {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return "", err
		}
		defer func() { _ = c.Close() }()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := io.WriteString(c, "GET / HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"); err != nil {
			return "", err
		}
		resp, err := http.ReadResponse(bufio.NewReader(c), nil)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		return string(b), err
	}

	me := uint32(os.Getuid())
	if body, err := get(serve(me)); err != nil || body != "ok" {
		t.Fatalf("own uid: %q, %v", body, err)
	}
	body, err := get(serve(me + 1))
	if err == nil {
		t.Fatalf("other uid: served %q", body)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		t.Fatalf("other uid: connection left open instead of dropped: %v", err)
	}
}
