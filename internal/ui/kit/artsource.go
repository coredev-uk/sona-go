package kit

import "github.com/coredev-uk/sona-go/internal/ui/store"

// ArtSource is where an ArtLoader gets covers from: the artwork cache in
// the app (*artwork.Cache), a generator in the component gallery, a fake in
// tests.
type ArtSource = artSource

// NewArtLoaderFrom is NewArtLoader over any ArtSource.
func NewArtLoaderFrom(src ArtSource, post store.Poster) *ArtLoader {
	return newArtLoader(src, post, 4)
}
