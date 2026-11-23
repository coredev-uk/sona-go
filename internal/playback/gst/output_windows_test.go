//go:build cgo

package gst

import "testing"

// A chosen endpoint plays while it can; gone, and when none is chosen, the
// output plays to the default endpoint, named (wasapi2sink stays where it
// opened, #312), or left to the sink ("") when Windows names none. Only
// wasapi2sink takes a device.
func TestOutputTarget(t *testing.T) {
	const (
		id  = "{0.0.0.00000000}.{2b411719-33dd-4bb0-8422-5e7032389a32}"
		def = "{0.0.0.00000000}.{7c3e2a10-9f1b-4c55-b0d2-1f6e8a4d9c01}"
	)
	for _, c := range []struct {
		sink, device string
		state        endpoint
		def          string
		want         string
	}{
		{"wasapi2sink", id, endpointActive, def, id},
		{"wasapi2sink", id, endpointGone, def, def},
		{"wasapi2sink", id, endpointGone, "", ""},
		{"wasapi2sink", id, endpointUnknown, def, id}, // Windows can't say: the sink finds out
		{"wasapi2sink", "", endpointActive, def, def},
		{"wasapi2sink", "", endpointActive, "", ""},
		{"wasapisink", id, endpointActive, def, ""},
		{"wasapisink", "", endpointActive, def, ""},
		{"directsoundsink", id, endpointActive, def, ""},
	} {
		asked := ""
		got := outputTarget(c.sink, c.device, func(d string) endpoint { asked = d; return c.state }, func() string { return c.def })
		if got != c.want {
			t.Errorf("outputTarget(%q, %q) with the endpoint %d and the default %q = %q, want %q", c.sink, c.device, c.state, c.def, got, c.want)
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
