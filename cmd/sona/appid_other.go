//go:build !linux

package main

import "log/slog"

func setApplicationID(*slog.Logger) {}
