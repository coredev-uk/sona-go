package artwork

import (
	"context"
	"errors"
	"image"
	"image/draw"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

// MosaicTiles is the number of artworks in a mosaic: a 2×2 grid.
const MosaicTiles = 4

// mosaicPrefix starts a mosaic template: the prefix, then the templates of
// its four tiles (top-left, top-right, bottom-left, bottom-right) separated
// by single spaces. A mosaic is addressed, cached and served like any other
// template, so every surface that shows artwork shows it unchanged.
const mosaicPrefix = "mosaic:"

// Mosaic picks the cover of a collection that has none of its own (a
// playlist) from the artwork templates of its items, in item order, and
// returns it as a template for URL, BackdropURL, Get and the rest of the
// cache:
//
//   - fewer than MosaicTiles distinct artworks (one album, a few albums):
//     the first artwork, as its own template;
//   - MosaicTiles or more: a 2×2 mosaic of the first four distinct ones.
//
// Artworks are distinct by template, which is one per album, so tracks of
// one album count once. Empty and invalid templates are skipped. The result
// is "" when no template is usable. A mosaic's key follows its four
// templates, so the cover is composed again exactly when the chosen
// artworks change.
func Mosaic(templates []string) string {
	var picked []string
	for _, t := range templates {
		t = strings.TrimSpace(t)
		if t == "" || strings.ContainsAny(t, " \t\r\n") || slices.Contains(picked, t) {
			continue
		}
		if _, _, err := parseTemplate(t); err != nil {
			continue
		}
		picked = append(picked, t)
		if len(picked) == MosaicTiles {
			break
		}
	}
	switch len(picked) {
	case 0:
		return ""
	case MosaicTiles:
		return mosaicPrefix + strings.Join(picked, " ")
	}
	return picked[0]
}

func isMosaic(tmpl string) bool { return strings.HasPrefix(tmpl, mosaicPrefix) }

// synthetic reports a template Sona makes itself, always square: a mosaic
// or a stored cover. Nothing is fetched for one.
func synthetic(tmpl string) bool {
	return isMosaic(tmpl) || strings.HasPrefix(tmpl, coverPrefix)
}

// mosaicTiles returns the tile templates of a normalised mosaic template.
func mosaicTiles(tmpl string) []string {
	return strings.Fields(strings.TrimPrefix(tmpl, mosaicPrefix))
}

// parseMosaic validates the part of a mosaic template after its prefix:
// exactly MosaicTiles templates of single tiles, each valid on its own.
func parseMosaic(rest string) (string, string, error) {
	parts := strings.Fields(rest)
	if len(parts) != MosaicTiles {
		return "", "", errors.New("artwork: a mosaic has four tiles")
	}
	for _, p := range parts {
		if isMosaic(p) {
			return "", "", errors.New("artwork: a mosaic cannot hold a mosaic")
		}
		if _, _, err := parseTemplate(p); err != nil {
			return "", "", err
		}
	}
	tmpl := mosaicPrefix + strings.Join(parts, " ")
	return tmpl, keyOf(tmpl), nil
}

// compose renders a mosaic template as a side×side JPEG: each tile is the
// centred square of its artwork's source (fetched like any artwork, at
// most once per standard size), scaled to half the side. Tiles are laid
// edge to edge in template order. ctx is the flight's, as fetch's.
func (c *Cache) compose(ctx context.Context, tmpl string, side int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*fetchTimeout)
	defer cancel()
	// Sizes are even, so four tiles fill the side exactly.
	half := side / 2
	canvas := image.NewRGBA(image.Rect(0, 0, side, side))
	g, ctx := errgroup.WithContext(ctx)
	for i, part := range mosaicTiles(tmpl) {
		g.Go(func() error {
			tile, err := c.tile(ctx, part, half)
			if err != nil {
				return err
			}
			x, y := i%2*half, i/2*half
			// The tiles cover disjoint parts of the canvas.
			draw.Draw(canvas, image.Rect(x, y, x+half, y+half), tile, image.Point{}, draw.Src)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return encodeJPEG(canvas, variantQuality)
}

// tile returns the artwork of template part as a size×size square.
func (c *Cache) tile(ctx context.Context, part string, size int) (*image.RGBA, error) {
	key, err := c.register(part)
	if err != nil {
		return nil, err
	}
	c.acquire(key)
	defer c.release(key)
	path, _, err := c.source(ctx, key, c.templateOf(key), dims{size, size})
	if err != nil {
		return nil, err
	}
	c.workSem <- struct{}{}
	defer func() { <-c.workSem }()
	img, err := c.decode(path)
	if err != nil {
		return nil, err
	}
	return cover(img, size), nil
}

// RemoveMosaics deletes every cached mosaic. Mosaics are the covers of the
// signed-in user's own playlists, so sign-out removes them with the
// artwork only that user's data references. It returns the bytes freed.
func (c *Cache) RemoveMosaics() (int64, error) { return c.removeCached(isMosaic) }

// RemovePatterns deletes every cached Apple-generated playlist cover, in
// any of its designs (Patterns): each carries a playlist's name, and the
// designs other than Apple's own are not in the library's rows, so
// sign-out removes them like the mosaics. It returns the bytes freed.
func (c *Cache) RemovePatterns() (int64, error) { return c.removeCached(isPattern) }

// removeCached deletes the cached artwork whose template satisfies match.
func (c *Cache) removeCached(match func(template string) bool) (int64, error) {
	c.mu.Lock()
	keys := make([]string, 0, len(c.keys))
	for k := range c.keys {
		keys = append(keys, k)
	}
	c.mu.Unlock()
	var refs []string
	for _, k := range keys {
		if b, err := os.ReadFile(filepath.Join(c.dir, k, templateFile)); err == nil && match(strings.TrimSpace(string(b))) {
			refs = append(refs, string(b))
		}
	}
	return c.Remove(refs)
}
