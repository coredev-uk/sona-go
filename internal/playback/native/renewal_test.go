package native

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// The CDM asks for a renewal before a licence expires (Apple's song and
// radio licences last 15 minutes; Chrome's CDM asks at 13). The engine
// answers it through Apple's licence server without touching playback;
// messages for sessions it does not own are ignored.
func TestLicenceRenewalRoundTrip(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	f := h.host(0)
	if n := h.fx.requests("/lic", ""); n != 1 {
		t.Fatalf("%d licence requests before renewal", n)
	}
	f.emit(cdmhost.Event{Kind: cdmhost.EventMessage, Session: "s-unknown", Type: int(widevine.LicenseRenewal), Data: []byte("renew")})
	f.emit(cdmhost.Event{Kind: cdmhost.EventMessage, Session: "s1", Type: int(widevine.LicenseRenewal), Data: []byte("renew")})
	eventually(t, "renewal sent to Apple", func() bool { return h.fx.requests("/lic", "") == 2 })
	time.Sleep(50 * time.Millisecond) // a request for the unknown session would land by now
	if n := h.fx.requests("/lic", ""); n != 2 {
		t.Fatalf("%d licence requests, want 2 (unknown session answered?)", n)
	}
	if st := h.e.Status(); st.State != playback.StatePlaying || st.ItemID != "A" {
		t.Fatalf("renewal disturbed playback: %+v", st)
	}
	if loads, _, _ := f.snapshot(); len(loads) != 1 {
		t.Fatalf("renewal reloaded the item: %+v", loads)
	}
}
