package config

// Crossfade overlaps consecutive tracks. It never applies between
// consecutive tracks of the same album: those play gapless, as mastered.
type Crossfade struct {
	Enabled bool `json:"enabled"`
	Seconds int  `json:"seconds"` // 1..12
}

// Crossfade lengths in seconds.
const (
	MinCrossfadeSeconds     = 1
	MaxCrossfadeSeconds     = 12
	defaultCrossfadeSeconds = 6
)

// normalized clamps the length to 1..12 s; an unset length (0) becomes
// the default.
func (c Crossfade) normalized() Crossfade {
	if c.Seconds == 0 {
		c.Seconds = defaultCrossfadeSeconds
	}
	c.Seconds = min(max(c.Seconds, MinCrossfadeSeconds), MaxCrossfadeSeconds)
	return c
}
