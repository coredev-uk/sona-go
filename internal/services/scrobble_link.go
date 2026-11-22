package services

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/mut"
)

// approvalLink is a one-use link on a loopback server that sends the browser
// to Last.fm's approval page. The page's address carries the approval token,
// and the address Sona hands the system is visible to every user of the
// computer as the browser's argv (/proc/<pid>/cmdline on Linux): with the
// token, another user could poll auth.getSession first and get the session
// key, or approve it with their own account. So the browser is given this
// link instead, which holds only a random name:
//
//   - On Linux the listener drops connections whose client socket another
//     user owns (mut.OwnerOnly, as the loopback sign-in does), so reading
//     the link gets another user nothing.
//   - The link works once; a second load is 410, so a link that reached
//     someone else is no use to them after the user's browser took it (and
//     if they were first, the user's browser is refused and the connect
//     fails, instead of silently going to another account).
//   - The Host header must be the loopback address, so a page cannot reach
//     the server through a rebound name.
//
// docs/security-review.md, "The Last.fm approval token was in the browser's
// command line".
type approvalLink struct {
	url  string // what the browser is given
	path string
	page string
	addr string
	srv  *http.Server
	used atomic.Bool
}

// newApprovalLink serves page, once, at a random path of a loopback port.
// The caller closes it when the approval ends.
func newApprovalLink(page string) (*approvalLink, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("the approval link: %w", err)
	}
	l := &approvalLink{page: page, addr: ln.Addr().String(), path: "/lastfm/" + rand.Text()}
	l.url = "http://" + l.addr + l.path
	l.srv = &http.Server{Handler: l, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = l.srv.Serve(mut.OwnerOnly(ln)) }()
	return l, nil
}

func (l *approvalLink) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	if r.Host != l.addr || r.Method != http.MethodGet ||
		subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte(l.path)) != 1 {
		http.NotFound(w, r)
		return
	}
	if !l.used.CompareAndSwap(false, true) {
		http.Error(w, "this link was already used; connect Last.fm again from Sona", http.StatusGone)
		return
	}
	http.Redirect(w, r, l.page, http.StatusFound)
}

// Close stops the server.
func (l *approvalLink) Close() error { return l.srv.Close() }
