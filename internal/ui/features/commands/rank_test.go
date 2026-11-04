package commands

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The ranking logic of v0.2.0's features/commands/rank.test.ts, over
// kit.RankCommands (the palette's ranking).

type cmd struct{ label, keywords string }

var ranked = []cmd{
	{"Show or hide the queue", "up next"},
	{"Next track", "skip forward"},
	{"Library · Albums", "library"},
	{"Albums of the year", ""},
	{"Play or pause", "resume stop"},
	{"Café playlists", ""},
}

func labels(q string) []string {
	var out []string
	for _, c := range kit.RankCommands(q, ranked, func(c cmd) string { return c.label }, func(c cmd) string { return c.keywords }) {
		out = append(out, c.label)
	}
	return out
}

func TestRankPutsAStartBeforeAContainedQuery(t *testing.T) {
	if got, want := labels("next"), []string{"Next track", "Show or hide the queue"}; !slices.Equal(got, want) {
		t.Errorf("next: %q, want %q", got, want)
	}
}

func TestRankWordStartsBeforeSubstringsAndKeywordsLast(t *testing.T) {
	if got, want := labels("alb"), []string{"Albums of the year", "Library · Albums"}; !slices.Equal(got, want) {
		t.Errorf("alb: %q, want %q", got, want)
	}
	if got, want := labels("skip"), []string{"Next track"}; !slices.Equal(got, want) {
		t.Errorf("skip: %q, want %q", got, want)
	}
}

func TestRankMatchesEveryTypedWordInAnyOrder(t *testing.T) {
	if got, want := labels("hide queue"), []string{"Show or hide the queue"}; !slices.Equal(got, want) {
		t.Errorf("hide queue: %q, want %q", got, want)
	}
	if got, want := labels("pause play"), []string{"Play or pause"}; !slices.Equal(got, want) {
		t.Errorf("pause play: %q, want %q", got, want)
	}
}

func TestRankFoldsCaseAndAccents(t *testing.T) {
	if got, want := labels("CAFE"), []string{"Café playlists"}; !slices.Equal(got, want) {
		t.Errorf("CAFE: %q, want %q", got, want)
	}
}

func TestRankFallsBackToLettersInOrder(t *testing.T) {
	if kit.MatchScore("ntr", "Next track", "") <= 0 {
		t.Error("ntr does not match Next track")
	}
	if kit.MatchScore("xyz", "Next track", "") != 0 {
		t.Error("xyz matches Next track")
	}
}

func TestRankKeepsGivenOrderForAnEmptyQuery(t *testing.T) {
	var want []string
	for _, c := range ranked {
		want = append(want, c.label)
	}
	if got := labels(""); !slices.Equal(got, want) {
		t.Errorf("empty: %q, want %q", got, want)
	}
}

func TestRankScoresFollowTheTable(t *testing.T) {
	for _, c := range []struct {
		q, label, kw string
		want         int
	}{
		{"next track", "Next track", "", 100},
		{"next", "Next track", "", 90},
		{"track", "Next track", "", 80},
		{"tra nex", "Next track", "", 70},
		{"ext", "Next track", "", 60},
		{"skip", "Next track", "skip forward", 40},
		{"ntr", "Next track", "", 20},
		{"xyz", "Next track", "", 0},
	} {
		if got := kit.MatchScore(c.q, c.label, c.kw); got != c.want {
			t.Errorf("MatchScore(%q, %q, %q) = %d, want %d", c.q, c.label, c.kw, got, c.want)
		}
	}
}
