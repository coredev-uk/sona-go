package lyricsync

import "github.com/coredev-uk/sona-go/internal/services"

// SongID is the catalogue ID of what plays when it has lyrics: "" for
// nothing, live radio or a music video (v0.2.0's lyricsSongId).
func SongID(cur *services.QueueItem) string {
	if cur == nil || cur.Live || cur.Video {
		return ""
	}
	return cur.CatalogID
}
