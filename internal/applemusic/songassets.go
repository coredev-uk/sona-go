package applemusic

import "context"

// SongAssets is what a catalog song says about its enhanced renditions
// (extend=extendedAssetUrls): its audio traits, and the master playlist
// that lists the Spatial Audio renditions (docs/apple-music-reference.md,
// "Spatial Audio and Dolby Atmos in enhancedHls").
type SongAssets struct {
	// AudioTraits are the song's badges: "lossless", "hi-res-lossless",
	// "lossy-stereo", "atmos", "spatial".
	AudioTraits []string
	// EnhancedHLS is the master playlist of every enhanced rendition; ""
	// when the song has none.
	EnhancedHLS string
}

// songAssetsResource is the part of a catalog song SongAssets reads.
type songAssetsResource struct {
	Attributes struct {
		AudioTraits       []string `json:"audioTraits"`
		ExtendedAssetURLs struct {
			EnhancedHLS string `json:"enhancedHls"`
		} `json:"extendedAssetUrls"`
	} `json:"attributes"`
}

// SongAssets returns catalog song id's audio traits and enhancedHls
// master playlist URL in storefront sf.
func (c *Client) SongAssets(ctx context.Context, sf, id string) (SongAssets, error) {
	r, err := AMPResource[songAssetsResource](ctx, c, sf, TypeSongs, id, AMPQuery{Extend: []string{"extendedAssetUrls"}})
	if err != nil {
		return SongAssets{}, err
	}
	return SongAssets{AudioTraits: r.Attributes.AudioTraits, EnhancedHLS: r.Attributes.ExtendedAssetURLs.EnhancedHLS}, nil
}
