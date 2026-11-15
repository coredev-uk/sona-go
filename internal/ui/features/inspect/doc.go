// Package inspect is the development build's media item inspector: right-click
// → Inspect on any item with a menu opens a plain panel with what Go mapped for
// the item and what Apple sent (InspectService, from Go's caches). Every file
// but this one carries the dev build tag, so a release build holds neither the
// action nor the panel (docs/ui-architecture.md, "Developer actions").
package inspect
