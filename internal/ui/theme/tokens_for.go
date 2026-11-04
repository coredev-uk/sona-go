package theme

// TokensFor returns the tokens of one appearance whatever the current mode
// is: what draws a picture of the other palette (the first-run setup's theme
// cards show the light and the dark window side by side). Main thread, like
// Current; the result is the cached value Current would return in that mode,
// so do not change it.
func TokensFor(dark bool) *Tokens {
	state.mu.Lock()
	defer state.mu.Unlock()
	i := 0
	if dark {
		i = 1
	}
	if t := state.tokens[i]; t != nil {
		return t
	}
	t, err := state.doc.Tokens(dark)
	if err != nil {
		panic(err) // Parse resolved both modes
	}
	state.tokens[i] = t
	return t
}
