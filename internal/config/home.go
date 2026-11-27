package config

// Home is the For You page's setting (Settings → General). Every value is
// valid, so there is nothing to normalise.
type Home struct {
	// Greeting shows "Good morning", "Good afternoon" or "Good evening"
	// above the page's heading. On by default.
	Greeting bool `json:"greeting"`
}
