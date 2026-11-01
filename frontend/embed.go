// Package frontend embeds the production frontend build for cmd/sona.
// `go tool wails3 build` (or `pnpm -C frontend build`) must run before
// `go build`.
package frontend

import "embed"

// Dist is frontend/dist, rooted at the package directory (paths start
// with "dist/").
//
//go:embed all:dist
var Dist embed.FS
