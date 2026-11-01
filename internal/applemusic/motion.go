package applemusic

// EditorialVideo is a resource's motion artwork (extend=editorialVideo),
// keyed by flavour: motionDetailSquare and motionSquareVideo1x1 (square:
// albums, playlists, stations), motionArtistSquare1x1 (an artist's square),
// motionDetailTall and motionTallVideo3x4 (3:4), motionArtistWide16x9 and
// others. Each video is an unencrypted HLS master playlist on Apple's media
// CDN.
type EditorialVideo map[string]MotionVideo

// MotionVideo is one motion artwork flavour: its HLS master playlist and
// the still it starts from.
type MotionVideo struct {
	Video        string   `json:"video"`
	PreviewFrame *Artwork `json:"previewFrame,omitempty"`
}

// squareFlavours are the square motion flavours, preferred first.
var squareFlavours = []string{"motionDetailSquare", "motionSquareVideo1x1", "motionArtistSquare1x1"}

// Square returns the HLS master playlist of the square motion artwork
// that stands in for the cover (or an artist's picture), "" when there is
// none.
func (v EditorialVideo) Square() string {
	for _, k := range squareFlavours {
		if u := v[k].Video; u != "" {
			return u
		}
	}
	return ""
}
