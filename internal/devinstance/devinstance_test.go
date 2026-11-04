package devinstance

import "testing"

func TestSanitize(t *testing.T) {
	for in, want := range map[string]string{
		"":         "",
		"feat-x y": "feat_x_y",
		"wt1":      "wt1",
		"a.b/c":    "a_b_c",
		"héllo":    "h_llo",
	} {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	long := make([]byte, 200)
	for i := range long {
		long[i] = 'a'
	}
	if got := sanitize(string(long)); len(got) != 64 {
		t.Errorf("long names are cut to 64, got %d", len(got))
	}
}
