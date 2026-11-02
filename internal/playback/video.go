package playback

import (
	"errors"
	"strings"
)

// Music videos are queue items like songs. Their playback ID is the
// catalog music video ID behind VideoPrefix, so the queue, the engine and
// every consumer of item IDs tell them from songs (Apple's song and video
// IDs are both plain numbers).
const VideoPrefix = "mv."

// VideoID is the playback ID of catalog music video id.
func VideoID(id string) string { return VideoPrefix + id }

// IsVideo reports whether playback ID id is a music video.
func IsVideo(id string) bool { return strings.HasPrefix(id, VideoPrefix) }

// VideoCatalogID is the catalog music video ID of playback ID id ("" when
// id is not a video).
func VideoCatalogID(id string) string {
	c, ok := strings.CutPrefix(id, VideoPrefix)
	if !ok {
		return ""
	}
	return c
}

// Music video failures an engine reports (wrapped) so the UI can say why.
var (
	// ErrVideoHDOnly: Apple offers no rendition of the video a software
	// Widevine CDM may play (SD H.264 without HDCP); its HD renditions
	// need a hardware-protected player.
	ErrVideoHDOnly = errors.New("playback: this video has no rendition a software CDM may play")
	// ErrVideoLicence: Apple refused the video's licence, or the key it
	// issued is not usable by this CDM (output-restricted).
	ErrVideoLicence = errors.New("playback: the video's licence was refused")
)
