package main

import (
	"maps"
	"testing"
)

func TestCompareKinds(t *testing.T) {
	cases := []struct {
		sona, apple string
		kind        string
		renames     map[string]string
	}{
		{"Shuffle", "Shuffle", matchExact, map[string]string{}},
		{"Disc {disc}", "Disc @@discNumber@@", matchExact, map[string]string{"discNumber": "disc"}},
		{"Add to playlist", "Add to Playlist", matchNormalised, map[string]string{}},
		{"Can’t play “{name}”…", "Can't play \"@@title@@\"...", matchNormalised, map[string]string{"title": "name"}},
		{"Loading…", "Loading...", matchNormalised, map[string]string{}},
		{"Done.", "Done", matchNormalised, map[string]string{}},
		{"Sign  in", "Sign\u00a0In", matchNormalised, map[string]string{}},
	}
	for _, c := range cases {
		apple, _ := appleMessage(appleBundle{"K": c.apple}, "K", false)
		kind, renames, ok := compare(mustSona(t, c.sona), apple)
		if !ok || kind != c.kind || !maps.Equal(renames, c.renames) {
			t.Errorf("compare(%q, %q) = %q %v %v; want %q %v", c.sona, c.apple, kind, renames, ok, c.kind, c.renames)
		}
	}
}

func TestCompareRejects(t *testing.T) {
	cases := []struct{ sona, apple string }{
		{"Play next", "Play later"},
		{"Play {name}", "Play"},                            // placeholder count
		{"{a} of {b}", "@@index@@ of @@index@@"},           // one Apple name for two arguments
		{"{name}, {artist}", "@@contentA@@, @@contentB@@"}, // no words to compare
		{"Add to playlist", "Add to the playlist"},
	}
	for _, c := range cases {
		apple, _ := appleMessage(appleBundle{"K": c.apple}, "K", false)
		if kind, _, ok := compare(mustSona(t, c.sona), apple); ok {
			t.Errorf("compare(%q, %q) = %q, want no match", c.sona, c.apple, kind)
		}
	}
}

func TestComparePluralForms(t *testing.T) {
	b := appleBundle{"N.one": "@@count@@ Song", "N.other": "@@count@@ Songs", "Z.zero": "No Songs", "Z.one": "@@count@@ Song", "Z.other": "@@count@@ Songs"}
	sona := mustSona(t, "{n, plural, one {# song} other {# songs}}")
	apple, _ := appleMessage(b, "N", true)
	kind, renames, ok := compare(sona, apple)
	if !ok || kind != matchNormalised || renames["count"] != "n" {
		t.Fatalf("compare = %q %v %v", kind, renames, ok)
	}
	// Different categories (Apple adds a zero form): no match.
	apple, _ = appleMessage(b, "Z", true)
	if _, _, ok := compare(sona, apple); ok {
		t.Fatal("matched despite different categories")
	}
}

func TestIndexMatchPrefersExactThenAppleMusicKeys(t *testing.T) {
	ix := indexBundle(appleBundle{
		"FUSE.Library":                  "Library",
		"ASE.Web.Platform.Shared.Lib":   "Library",
		"ASE.Web.Music.Side.Library":    "Library",
		"ASE.Web.Music.Lower":           "library",
		"ASE.Web.Music.NoOfSongs.one":   "@@count@@ Song",
		"ASE.Web.Music.NoOfSongs.other": "@@count@@ Songs",
	})
	got := ix.match(mustSona(t, "Library"))
	var keys []string
	for _, c := range got {
		keys = append(keys, c.Key)
	}
	want := []string{"ASE.Web.Music.Side.Library", "ASE.Web.Platform.Shared.Lib", "FUSE.Library", "ASE.Web.Music.Lower"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
	// Plural form keys are matched as plurals only, never as plain text.
	if got := ix.match(mustSona(t, "{count} Song")); len(got) != 0 {
		t.Fatalf("plain match on a plural form: %v", got)
	}
	if got := ix.match(mustSona(t, "{n, plural, one {# Song} other {# Songs}}")); len(got) != 1 || !got[0].Plural || got[0].Kind != matchExact {
		t.Fatalf("plural match = %+v", got)
	}
}

func TestMatchAllSkipsMappedAndRejected(t *testing.T) {
	sona := &sonaCatalogue{parsed: map[msgKey]message{}}
	for _, id := range []string{"Play", "Pause", "Home", "Queue"} {
		e := &poEntry{ID: id}
		sona.entries = append(sona.entries, e)
		sona.parsed[e.key()] = mustSona(t, id)
	}
	ix := indexBundle(appleBundle{"A.Play": "Play", "A.Pause": "Pause", "A.Home": "Home", "B.Home": "Home"})
	res := matchAll(sona, ix,
		[]mapping{{Message: "Play", Key: "A.Play"}},
		[]rejection{{Message: "Home", Key: "A.Home", Reason: "keyboard key"}, {Message: "Home", Key: "B.Home", Reason: "keyboard key"}})
	if res.exact != 3 || res.rejectedHits != 1 {
		t.Fatalf("exact %d, rejectedHits %d", res.exact, res.rejectedHits)
	}
	if len(res.proposals) != 1 || res.proposals[0].Message != "Pause" || res.proposals[0].Key != "A.Pause" {
		t.Fatalf("proposals = %+v", res.proposals)
	}
}
