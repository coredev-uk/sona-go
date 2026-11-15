package services

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// brickFixture is a brick tile (386) of one apple-curator whose editorial
// artwork flavours are given as key → "WxH#bgColor".
func brickFixture(t *testing.T, flavours map[string]string) (applemusic.Resources, applemusic.Node) {
	t.Helper()
	ea := map[string]any{}
	for k, v := range flavours {
		size, bg, _ := strings.Cut(v, "#")
		w, h, _ := strings.Cut(size, "x")
		ea[k] = map[string]any{
			"url":     "https://is1-ssl.mzstatic.com/image/thumb/Features/v4/x/" + k + ".png/{w}x{h}{c}.{f}",
			"width":   atoi(t, w),
			"height":  atoi(t, h),
			"bgColor": bg,
		}
	}
	node := func(typ, id string, attrs map[string]any, rels map[string]any) applemusic.Node {
		raw, err := json.Marshal(map[string]any{"id": id, "type": typ, "attributes": attrs, "relationships": rels})
		if err != nil {
			t.Fatal(err)
		}
		var n applemusic.Node
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	curator := node("apple-curators", "c1", map[string]any{"name": "Club", "editorialArtwork": ea}, nil)
	tile := node("editorial-elements", "t1", map[string]any{"editorialElementKind": applemusic.ElementBrickItem},
		map[string]any{"contents": map[string]any{"data": []map[string]string{{"id": "c1", "type": "apple-curators"}}}})
	return applemusic.Resources{"apple-curators": {"c1": curator}}, tile
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	var n int
	if err := json.Unmarshal([]byte(s), &n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A brick takes the wide flavours in wideKeys' order (subscriptionHero, the
// one music.apple.com's brick profile uses, first), and a curator with only a
// square subscriptionCover still gets a banner: its square shape, which the
// row then crops to the brick's frame.
func TestBrickKeyChoice(t *testing.T) {
	m := newMapper(t)
	for _, tc := range []struct {
		name      string
		flavours  map[string]string
		wantKey   string // the flavour whose art is the banner; "" for none
		wantLarge string // suffix of the banner's Large URL
		wantColor string
	}{
		{"hero beats cover", map[string]string{"subscriptionCover": "4320x1080#2a2a2a", "subscriptionHero": "4320x1080#000000", "bannerUber": "4320x1080#2a2a2a"},
			"subscriptionHero", "/1000x250.jpg", "#000000"},
		{"only a square cover", map[string]string{"subscriptionCover": "1080x1080#112233", "brandLogo": "1080x1080#f4f4f4"},
			"subscriptionCover", "/1000x1000.jpg", "#112233"},
		{"no wide flavour", map[string]string{"brandLogo": "1080x1080#f4f4f4"}, "", "", ""},
	} {
		res, tile := brickFixture(t, tc.flavours)
		it, ok := m.brick(res, tile)
		if !ok {
			t.Fatalf("%s: brick dropped", tc.name)
		}
		if tc.wantKey == "" {
			if it.Wide != (Art{}) {
				t.Errorf("%s: wide = %+v, want none", tc.name, it.Wide)
			}
			continue
		}
		if !strings.Contains(it.Wide.Large, "/art/") || !strings.HasSuffix(it.Wide.Large, tc.wantLarge) || it.Color != tc.wantColor {
			t.Errorf("%s: wide %q colour %q, want …%s on %s (%s)", tc.name, it.Wide.Large, it.Color, tc.wantLarge, tc.wantColor, tc.wantKey)
		}
	}
}
