package config

// Home is the For You page's settings (Settings → General). Every value is
// valid, so there is nothing to normalise.
type Home struct {
	// Greeting shows "Good morning", "Good afternoon" or "Good evening"
	// as the page's opening line. On by default.
	Greeting bool `json:"greeting"`
	// UseName adds the person's first name from their Apple Music profile
	// to the greeting, when there is one. On by default; it counts only
	// while Greeting is on.
	UseName bool `json:"useName"`
}
