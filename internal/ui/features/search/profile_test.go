package search

import "testing"

func TestProfileShowsWhoTheyAreAndTheirPlaylists(t *testing.T) {
	r := newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.verified = true }})
	r.tt.Until("the profile", r.tt.Shows("Taylor Listener", "@taylor", "Public playlists", "Shared Mix 1", "Shared Mix 2"))
	if _, ok := r.tt.Find("Verified"); !ok {
		t.Error("a verified profile lacks the badge")
	}
	if r.tt.HasText("Private profile") {
		t.Error("a public profile says it is private")
	}
	r.click("Shared Mix 2")
	if got := r.loc(); got != "/playlist/pl.u-2" {
		t.Errorf("a playlist opened %q", got)
	}
}

func TestProfileWithoutVerifiedBadge(t *testing.T) {
	r := newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true})
	r.tt.Until("the profile", r.tt.Shows("Taylor Listener"))
	if _, ok := r.tt.Find("Verified"); ok {
		t.Error("an unverified profile has the badge")
	}
}

func TestProfilePrivateMarkAndEmptyPlaylists(t *testing.T) {
	r := newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.private, a.playlists = true, 0 }})
	r.tt.Until("the empty state", r.tt.Shows("@taylor", "Private profile", "No public playlists",
		"Taylor Listener keeps their profile private, so only followers see what they share."))
	if r.tt.HasText("Public playlists") {
		t.Error("an empty profile has the shelf")
	}

	r2 := newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.playlists = 0 }})
	r2.tt.Until("the empty state", r2.tt.Shows("No public playlists", "Taylor Listener hasn't shared any playlists on their profile."))
	if r2.tt.HasText("Private profile") {
		t.Error("a public profile says it is private")
	}
}

func TestProfileLoadingThenError(t *testing.T) {
	r := newPageRig(t, "/profile/sp.1", rigOpts{signedIn: true, setup: func(a *pageApple) { a.fail["/v1/social/gb/social-profiles"] = 500 }})
	if _, ok := r.tt.Find("Loading"); !ok && !r.tt.HasText("Couldn't load this profile") {
		t.Error("neither a skeleton nor an error on the first frame")
	}
	r.tt.Until("the error", r.tt.Shows("Couldn't load this profile"))
	r.apple.setFail("/v1/social/gb/social-profiles", 0)
	r.click("Try again")
	r.tt.Until("the profile", r.tt.Shows("Taylor Listener"))
}
