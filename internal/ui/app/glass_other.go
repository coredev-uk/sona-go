//go:build !darwin

package app

import (
	"log/slog"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func glassWindow(*mygo.WindowOptions)       {}
func glassMini(*mygo.WindowOptions)         {}
func glassReport(*ui.Context, *slog.Logger) {}
