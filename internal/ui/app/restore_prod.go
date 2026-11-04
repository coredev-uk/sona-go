//go:build !dev

package app

import "github.com/egoist/mygo"

// devHook is a development build's restore and live reload (restore_dev.go):
// nothing in a production build.
func (a *App) devHook(*mygo.Window) {}
