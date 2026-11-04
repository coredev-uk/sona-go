package concerts

import (
	"net/url"
	"testing"
)

func TestParamsKeepWhatTheyCanRead(t *testing.T) {
	got := paramsFrom(url.Values{
		"when": {"dates"}, "from": {"2027-04-01"}, "to": {"2027-04-30"}, "genre": {"20"}, "yours": {"true"},
	})
	want := params{when: "dates", from: "2027-04-01", to: "2027-04-30", genre: "20", yours: true}
	if got != want {
		t.Errorf("params = %+v, want %+v", got, want)
	}
}

func TestParamsDropWhatTheyCannot(t *testing.T) {
	// From a saved or hand-typed address.
	got := paramsFrom(url.Values{
		"when": {"tomorrow"}, "from": {"April"}, "to": {"2027-4-1"}, "genre": {"rock"}, "yours": {"no"},
	})
	if got != (params{}) {
		t.Errorf("params = %+v, want none", got)
	}
}

func TestParamsLocationWritesOnlyWhatIsSet(t *testing.T) {
	if got := (params{}).location(); got != "/concerts" {
		t.Errorf("empty location = %q", got)
	}
	p := params{when: "dates", from: "2027-04-01", to: "2027-04-30", genre: "20", yours: true}
	if got := p.location(); got != "/concerts?from=2027-04-01&genre=20&to=2027-04-30&when=dates&yours=true" {
		t.Errorf("location = %q", got)
	}
	if again := paramsFrom(mustQuery(t, p.location())); again != p {
		t.Errorf("round trip = %+v", again)
	}
}

func mustQuery(t *testing.T, loc string) url.Values {
	t.Helper()
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	return u.Query()
}
