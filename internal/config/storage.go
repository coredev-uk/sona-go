package config

// Artwork cache size bounds, in MiB.
const (
	DefaultArtCacheMiB = 512
	MinArtCacheMiB     = 64
	MaxArtCacheMiB     = 16 << 10
)

// Storage is the local cache configuration (Settings → Storage).
type Storage struct {
	// ArtCacheMiB caps the artwork disk cache; least recently used artwork
	// is evicted above it.
	ArtCacheMiB int `json:"artCacheMiB"`
}

// normalized clamps the artwork cache size; 0 (absent) is the default.
func (s Storage) normalized() Storage {
	if s.ArtCacheMiB == 0 {
		s.ArtCacheMiB = DefaultArtCacheMiB
	}
	s.ArtCacheMiB = min(max(s.ArtCacheMiB, MinArtCacheMiB), MaxArtCacheMiB)
	return s
}
