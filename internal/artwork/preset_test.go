package artwork

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"slices"
	"strings"
	"testing"
)

func TestPresetsAreOnePerStyleAndValid(t *testing.T) {
	for name, palette := range map[string][]Color{
		"three colours": {{0x3a, 0x7b, 0xd5}, {0xd5, 0xb4, 0x3a}, {0xb0, 0x30, 0x40}},
		"two colours":   {{0x3a, 0x7b, 0xd5}, {0xd5, 0xb4, 0x3a}},
		"one colour":    {{0x3a, 0x7b, 0xd5}},
		"none":          nil,
		"unreadable":    {{}, {}},
		"more than 3":   {{1, 2, 200}, {200, 2, 1}, {2, 200, 1}, {200, 200, 1}},
	} {
		t.Run(name, func(t *testing.T) {
			got := Presets(palette)
			if len(got) != len(PresetStyles) {
				t.Fatalf("%d presets, want %d", len(got), len(PresetStyles))
			}
			seen := map[string]bool{}
			for i, tm := range got {
				if !IsPreset(tm) {
					t.Fatalf("%q is not a valid preset", tm)
				}
				if PresetStyle(tm) != PresetStyles[i] {
					t.Errorf("preset %d has style %q, want %q", i, PresetStyle(tm), PresetStyles[i])
				}
				if seen[tm] {
					t.Errorf("%q is offered twice", tm)
				}
				seen[tm] = true
			}
		})
	}
}

func TestPresetsTakeThePlaylistsColours(t *testing.T) {
	a, b, c := Color{0x20, 0x60, 0xc0}, Color{0xc0, 0x60, 0x20}, Color{0x40, 0xa0, 0x40}
	got := Presets([]Color{a, b, c})
	want := "preset:aurora:" + a.hex6() + ":" + b.hex6()
	if got[0] != want {
		t.Errorf("first preset = %q, want %q", got[0], want)
	}
	// The same playlist gives the same covers; another gives other ones.
	if !slices.Equal(got, Presets([]Color{a, b, c})) {
		t.Error("Presets is not deterministic")
	}
	if slices.Equal(got, Presets([]Color{c, b, a})) {
		t.Error("another palette gave the same presets")
	}
	// An unreadable image's black swatches are not a palette.
	if !slices.Equal(Presets([]Color{{}, {}}), Presets(nil)) {
		t.Error("black swatches were taken as the playlist's colours")
	}
}

func TestPresetTemplateValidation(t *testing.T) {
	good := "preset:aurora:3a7bd5:d5b43a"
	if !IsPreset(good) {
		t.Fatalf("%q was refused", good)
	}
	for name, tm := range map[string]string{
		"unknown style":    "preset:rainbow:3a7bd5:d5b43a",
		"one colour":       "preset:aurora:3a7bd5",
		"three colours":    "preset:aurora:3a7bd5:d5b43a:ffffff",
		"short hex":        "preset:aurora:3a7bd:d5b43a",
		"upper case hex":   "preset:aurora:3A7BD5:d5b43a",
		"not hex":          "preset:aurora:3a7bdz:d5b43a",
		"hash":             "preset:aurora:#3a7bd5:d5b43a",
		"no style":         "preset::3a7bd5:d5b43a",
		"other prefix":     "presets:aurora:3a7bd5:d5b43a",
		"space":            "preset:aurora:3a7bd5: d5b43a",
		"empty":            "preset:",
		"path traversal":   "preset:../../etc:3a7bd5:d5b43a",
		"trailing newline": good + "\n",
	} {
		if IsPreset(tm) {
			t.Errorf("%s: %q was accepted", name, tm)
		}
		if _, _, err := parseTemplate(tm); err == nil && !strings.HasSuffix(tm, "\n") {
			t.Errorf("%s: %q parsed as a template", name, tm)
		}
	}
}

func TestPresetsAreServedLikeAnyArtwork(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	for _, tm := range Presets([]Color{{0x3a, 0x7b, 0xd5}, {0xd5, 0xb4, 0x3a}}) {
		for _, side := range []int{64, 200, 600} {
			b, err := c.Get(context.Background(), tm, side, side)
			if err != nil {
				t.Fatalf("%s at %d: %v", tm, side, err)
			}
			if got := bounds(t, b); got.Dx() != side || got.Dy() != side {
				t.Errorf("%s at %d: %v", tm, side, got)
			}
		}
		if u := c.URL(tm, 100, 100); u == "" {
			t.Errorf("no URL for %s", tm)
		}
		if _, err := c.Backdrop(context.Background(), tm); err != nil {
			t.Errorf("backdrop of %s: %v", tm, err)
		}
		cols, err := c.DominantColors(context.Background(), tm)
		if err != nil || len(cols) < 2 {
			t.Errorf("colours of %s = %v, %v", tm, cols, err)
		}
	}
}

// A preset is drawn from its template alone: the same template is the same
// picture, and the styles are different pictures that are not flat.
func TestPresetStylesDrawDistinctPictures(t *testing.T) {
	pics := map[string][]byte{}
	for _, tm := range Presets([]Color{{0x3a, 0x7b, 0xd5}, {0xd5, 0xb4, 0x3a}, {0xb0, 0x30, 0x40}}) {
		a, err := renderPreset(context.Background(), tm, 96)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := renderPreset(context.Background(), tm, 96)
		if !bytes.Equal(a, b) {
			t.Errorf("%s drew two pictures", tm)
		}
		img, err := jpeg.Decode(bytes.NewReader(a))
		if err != nil {
			t.Fatal(err)
		}
		if spread(img) < 40 {
			t.Errorf("%s is nearly flat", tm)
		}
		for other, ob := range pics {
			if bytes.Equal(a, ob) {
				t.Errorf("%s and %s drew the same picture", tm, other)
			}
		}
		pics[tm] = a
	}
}

// spread is the largest difference in luma between two pixels.
func spread(img image.Image) int {
	lo, hi := 255, 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			l := int(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y)
			lo, hi = min(lo, l), max(hi, l)
		}
	}
	return hi - lo
}
