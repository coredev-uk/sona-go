package config

// Replace-queue behaviour when playing a collection would discard songs
// the user added to the queue by hand.
const (
	ReplaceUndo    = "undo"    // replace at once and offer Undo
	ReplaceConfirm = "confirm" // ask before replacing
)

// Queue is the play queue's behaviour (Settings → Playback).
type Queue struct {
	// ReplaceConfirm is ReplaceUndo (the default) or ReplaceConfirm.
	ReplaceConfirm string `json:"replaceConfirm"`
}

func (q Queue) normalized() Queue {
	if q.ReplaceConfirm != ReplaceConfirm {
		q.ReplaceConfirm = ReplaceUndo
	}
	return q
}
