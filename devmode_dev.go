//go:build !production

package main

// devMode enables text logging to stderr and asset-server request logs.
// Production builds (`-tags production`, as `wails3 build` passes) turn it off.
const devMode = true
