//go:build !linux

package shell

import "github.com/wailsapp/wails/v3/pkg/application"

// guardKeyboard works around WebKitGTK keyboard traps; other platforms'
// web views deliver Shift+F10, the Menu key and Tab to the page as they are.
func guardKeyboard(*application.WebviewWindow) {}
