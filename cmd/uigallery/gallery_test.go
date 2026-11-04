package main

import (
	"bytes"
	"context"
	"image/jpeg"
	"log/slog"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Every section builds, light and dark, without a panic (a repeated key, an
// unknown token) and shows its title: a kit PR's section is checked by
// being registered.
func TestSectionsBuild(t *testing.T) {
	if len(sections) == 0 {
		t.Fatal("no sections registered")
	}
	for _, dark := range []bool{false, true} {
		for _, s := range sections {
			name := s.name
			if dark {
				name += " dark"
			}
			t.Run(name, func(t *testing.T) {
				t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
				g := &gallery{dark: dark, selected: s.name, scale: 1, scrolls: map[string]*ui.ScrollState{}}
				// Covers load on worker goroutines and come back through the
				// Poster. The window runs what they post on its main thread;
				// here nothing runs it, so a section is built with its covers
				// not yet loaded.
				g.art = kit.NewArtLoaderFrom(coverSource{}, func(func()) {})
				t.Cleanup(g.art.Close)
				g.log = slog.New(slog.DiscardHandler)
				tt := ui.NewTester(g.view, 1100, 800)
				tt.Frame()
				if !tt.HasText(s.name) {
					t.Fatalf("the section's title %q is not on screen: %v", s.name, tt.Texts())
				}
				if tt.Image() == nil {
					t.Fatal("no frame")
				}
			})
		}
	}
}

func TestRegisterRejectsDuplicates(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a second section of the same name must panic")
		}
	}()
	register(section{name: sections[0].name, build: func(*ui.Context, *gallery) {}})
}

func TestCoverSource(t *testing.T) {
	var src coverSource
	data, err := src.Get(context.Background(), coverURL(7, 0, false, 0), 120, 80)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 120 || b.Dy() != 80 {
		t.Fatalf("cover is %v, want 120x80", b)
	}
	other, _ := src.Get(context.Background(), coverURL(8, 0, false, 0), 120, 80)
	if bytes.Equal(data, other) {
		t.Fatal("two covers are the same picture")
	}
	if again, _ := src.Get(context.Background(), coverURL(7, 0, false, 0), 120, 80); !bytes.Equal(data, again) {
		t.Fatal("a cover changes between asks")
	}
	if _, err := src.Get(context.Background(), coverURL(1, 0, true, 0), 10, 10); err == nil {
		t.Fatal("fail=1 must fail")
	}
	// Any src has a cover, and the loader may ask for a src of another shape.
	if _, err := src.Get(context.Background(), "https://example.invalid/a/{w}x{h}.jpg", 10, 10); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := src.Get(ctx, coverURL(1, time.Minute, false, 0), 10, 10); err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("a delayed cover must stop with its context: %v", err)
	}
}
