package i18n

import "testing"

func TestSplitLink(t *testing.T) {
	for _, tc := range []struct {
		msg, before, link, after string
		ok                       bool
	}{
		{"Change them in <0>Settings</0>.", "Change them in ", "Settings", ".", true},
		{"Copy it from <0>your settings<1/></0>. Done.", "Copy it from ", "your settings", ". Done.", true},
		{"<0>All</0>", "", "All", "", true},
		{"no markers", "no markers", "", "", false},
		{"unclosed <0>link", "unclosed ", "link", "", false},
	} {
		before, link, after, ok := SplitLink(tc.msg)
		if before != tc.before || link != tc.link || after != tc.after || ok != tc.ok {
			t.Errorf("SplitLink(%q) = %q, %q, %q, %v; want %q, %q, %q, %v", tc.msg, before, link, after, ok, tc.before, tc.link, tc.after, tc.ok)
		}
	}
}

func TestStripTags(t *testing.T) {
	if got, want := StripTags("Pauses at <0>5:00</0>."), "Pauses at 5:00."; got != want {
		t.Errorf("StripTags = %q, want %q", got, want)
	}
}
