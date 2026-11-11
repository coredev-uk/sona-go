// Package gstbundle points GStreamer at the copy that ships beside the
// executable, on Windows (build/windows/package.sh lays it out). Import it for
// its side effect from every program that plays (cmd/sona, cmd/playspike): its
// init runs before anything starts GStreamer, and the CDM host child, which is
// the same executable, inherits the environment it sets.
//
// A build with no GStreamer beside it (a development build, other systems)
// is left alone and keeps whatever its environment says. macOS does the same
// in cmd/sona/bundle_darwin.go; Linux uses the system's GStreamer.
package gstbundle
