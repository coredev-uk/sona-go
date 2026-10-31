package playback

import "testing"

func TestPreviewEnv(t *testing.T) {
	for _, c := range []struct {
		v      string
		on, ok bool
	}{
		{"", false, true},
		{"1", true, true},
		{"true", true, true},
		{"0", false, true},
		{"false", false, true},
		{"yes", false, false},
	} {
		on, err := PreviewEnv(c.v)
		if on != c.on || (err == nil) != c.ok {
			t.Errorf("PreviewEnv(%q) = %v, %v; want %v, ok %v", c.v, on, err, c.on, c.ok)
		}
	}
}
