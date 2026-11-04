// Package build embeds the app icons cmd/sona hands to the UI.
package build

import _ "embed" // for go:embed

// AppIcon is the window and application icon.
//
//go:embed appicon.png
var AppIcon []byte

// TrayIcon is the tray icon: a small PNG, since StatusNotifierItem sends
// pixels over D-Bus.
//
//go:embed linux/icons/hicolor/64x64/apps/sona.png
var TrayIcon []byte
