package config

// MotionArtwork is the motion cover setting (Settings → Appearance). The
// UI plays motion covers only while Enabled, reduce effects is off,
// the system does not ask for reduced motion and the machine is on mains
// power; there is nothing to normalise.
type MotionArtwork struct {
	// Enabled plays Apple's looping motion covers in album and playlist
	// headers and on the now-playing view. On by default.
	Enabled bool `json:"enabled"`
}
