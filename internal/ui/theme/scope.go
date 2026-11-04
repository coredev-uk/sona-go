package theme

import "github.com/egoist/mygo/ui"

// scopes are the colour overrides in force, innermost last. Views are built
// on the main thread, and a scope is only open while its children are being
// built, so the stack is as deep as the nesting.
var scopes []map[string]ui.Color

// Scope builds children with some colour tokens replaced: a page that is
// printed in its cover's colours (v0.2.0 redeclared custom properties on the
// sleeve, and everything inside followed). C looks a token up in the
// innermost scope that names it, then outward, then in the theme's tokens.
// The overrides are resolved where the token is read, so only colours read
// while children builds are affected: a colour fetched inside a Draw
// callback (after the scope has closed) is the theme's own. The map is read,
// never changed: build it once per cover and theme (internal/ui/theme/palette),
// not per frame. A nil or empty map just runs children.
func Scope(over map[string]ui.Color, children func()) {
	if len(over) == 0 {
		children()
		return
	}
	state.mu.Lock()
	scopes = append(scopes, over)
	state.mu.Unlock()
	defer func() {
		state.mu.Lock()
		scopes = scopes[:len(scopes)-1]
		state.mu.Unlock()
	}()
	children()
}

// scoped is the override of a token, if a scope has one.
func scoped(name string) (ui.Color, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for i := len(scopes) - 1; i >= 0; i-- {
		if c, ok := scopes[i][name]; ok {
			return c, true
		}
	}
	return ui.Color{}, false
}
