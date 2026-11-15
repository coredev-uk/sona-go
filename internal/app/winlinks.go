package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// WindowsLinkHandler is the default app for the Apple Music link schemes
// (LinkSchemes) on Windows: services.LinkHandler there. Its parts are the
// system's (cmd/sona/linkhandler_windows.go wires them), so the rules stay
// portable and tested everywhere.
//
// A scheme opens in Sona when all of these hold:
//
//   - the user's class HKCU\Software\Classes\<scheme> runs this executable
//     (Registered), which MyGo's RegisterURLScheme writes and the installer
//     writes for its folder;
//   - the user's choice for the scheme (UrlAssociations\<scheme>\UserChoice,
//     which Windows keeps when the user picked an app, and which no app may
//     write) is absent or names that class, whose ProgId is the scheme;
//   - Windows doesn't resolve the scheme to a packaged app (Apple Music from
//     the Microsoft Store declares music:// and itms:// in its package), which
//     the class can't override.
type WindowsLinkHandler struct {
	// Registered reports whether the user's class for scheme runs this
	// executable.
	Registered func(scheme string) bool
	// Register points the user's class for scheme at this executable.
	Register func(scheme string) error
	// Choice is the ProgId of the user's choice for scheme; "" for none.
	Choice func(scheme string) string
	// Handler is the app Windows opens scheme's links with: its display name
	// ("" when unknown) and whether it is a packaged app.
	Handler func(scheme string) (name string, packaged bool)
}

// Default reports whether every scheme opens in this Sona, else the name of
// the app that opens the first one that doesn't ("" when unknown).
func (h WindowsLinkHandler) Default() (bool, string, error) {
	for _, scheme := range LinkSchemes {
		name, packaged := h.Handler(scheme)
		choice := h.Choice(scheme)
		if h.Registered(scheme) && !packaged && (choice == "" || strings.EqualFold(choice, scheme)) {
			continue
		}
		return false, name, nil
	}
	return true, "", nil
}

// MakeDefault points every scheme's class at this executable. It can't
// overrule a choice the user made in Windows' settings, nor a packaged app:
// Default still names that app afterwards, and the user changes it in
// Settings → Apps → Default apps.
func (h WindowsLinkHandler) MakeDefault(ctx context.Context) error {
	var errs []error
	for _, scheme := range LinkSchemes {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := h.Register(scheme); err != nil {
			errs = append(errs, fmt.Errorf("windows link handler: %s: %w", scheme, err))
		}
	}
	return errors.Join(errs...)
}
