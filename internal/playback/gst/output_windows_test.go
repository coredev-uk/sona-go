//go:build cgo

package gst

import "testing"

// A chosen endpoint that is gone plays to the default ("") until it is
// back; only wasapi2sink takes a device.
func TestOutputTarget(t *testing.T) {
	const id = "{0.0.0.00000000}.{2b411719-33dd-4bb0-8422-5e7032389a32}"
	for _, c := range []struct {
		sink, device string
		state        endpoint
		want         string
	}{
		{"wasapi2sink", id, endpointActive, id},
		{"wasapi2sink", id, endpointGone, ""},
		{"wasapi2sink", id, endpointUnknown, id}, // Windows can't say: the sink finds out
		{"wasapi2sink", "", endpointActive, ""},
		{"wasapisink", id, endpointActive, ""},
		{"directsoundsink", id, endpointActive, ""},
	} {
		asked := ""
		got := outputTarget(c.sink, c.device, func(d string) endpoint { asked = d; return c.state })
		if got != c.want {
			t.Errorf("outputTarget(%q, %q) with the endpoint %d = %q, want %q", c.sink, c.device, c.state, got, c.want)
		}
		if asked != "" && asked != c.device {
			t.Errorf("asked about %q, not the chosen %q", asked, c.device)
		}
	}
}

// Windows itself answers for an ID no endpoint has: gone, so the output
// plays to the default, and the endpoint watch starts.
func TestEndpointStateOfAnUnknownID(t *testing.T) {
	routeEpoch()
	switch s := endpointState("{0.0.0.00000000}.{00000000-0000-0000-0000-000000000000}"); s {
	case endpointGone:
	case endpointUnknown:
		t.Skip("the endpoint API is unavailable here")
	default:
		t.Fatalf("an endpoint that does not exist is %d", s)
	}
	if s := endpointState("not an endpoint ID"); s != endpointGone && s != endpointUnknown {
		t.Fatalf("a malformed ID is %d", s)
	}
}
