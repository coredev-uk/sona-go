//go:build dev

package main

// devMode is a development build (-tags dev): text logging to stderr, the
// session starts private, and SONA_INSPECTOR=1 opens MyGo's inspector.
const devMode = true
