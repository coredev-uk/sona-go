//go:build darwin

package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// writeWallpaper draws the spike's desktop picture: broad diagonal colour,
// big discs and bars of strong colour, no text. The runner's own wallpaper
// is nearly flat, which shows next to nothing through a pane of glass; this
// is the busy backdrop a real desktop is, not a measurement of anything.
func writeWallpaper(file string) error {
	const w, h = 2048, 1536
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	type disc struct {
		x, y, r float64
		c       color.RGBA
	}
	discs := []disc{
		{0.18, 0.25, 0.20, color.RGBA{255, 214, 10, 255}},
		{0.62, 0.15, 0.16, color.RGBA{255, 55, 95, 255}},
		{0.85, 0.55, 0.24, color.RGBA{10, 132, 255, 255}},
		{0.30, 0.78, 0.22, color.RGBA{48, 209, 88, 255}},
		{0.70, 0.88, 0.14, color.RGBA{191, 90, 242, 255}},
		{0.05, 0.60, 0.10, color.RGBA{255, 159, 10, 255}},
	}
	for y := range h {
		for x := range w {
			fx, fy := float64(x)/w, float64(y)/h
			t := (fx + fy) / 2
			r := 40 + 180*math.Abs(math.Sin(3*t*math.Pi))
			g := 60 + 150*math.Abs(math.Sin(2*t*math.Pi+1))
			b := 120 + 120*math.Abs(math.Cos(2.5*t*math.Pi))
			c := color.RGBA{uint8(r), uint8(g), uint8(b), 255}
			for _, d := range discs {
				if math.Hypot((fx-d.x)*w, (fy-d.y)*w) < d.r*w {
					c = d.c
				}
			}
			// White and black bars, so edges of high contrast are behind the glass too.
			if int(fy*h)%420 < 36 {
				c = color.RGBA{250, 250, 250, 255}
			}
			if int(fx*w+fy*h)%610 < 40 {
				c = color.RGBA{20, 20, 24, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}
