// Package inspect is the media item inspector: right-click → Inspect on any
// item with a menu opens a panel with what Go mapped for the item, what Apple
// sent and what an add-on is told of it (InspectService, from Go's caches). A
// development build offers it always; a release build only while Settings →
// Diagnostics → Developer tools is on, and it applies at once: the action is
// registered and unregistered through Env.Actions (docs/ui-architecture.md,
// "Developer actions").
package inspect
