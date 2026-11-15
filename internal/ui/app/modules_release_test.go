//go:build !dev

package app

// devModules are the modules a development build adds: none in a release
// build, which registers no inspector (TestRegisteredModulesAreEachOnce…
// compares the registered modules with this list).
var devModules []string
