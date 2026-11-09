// Package demo is a minimal feature used to exercise the agent task
// environment end to end (edit, check, test, commit). Nothing imports it.
package demo

import "strings"

// Greeting returns a greeting for name, or for "listener" when name is blank.
func Greeting(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "listener"
	}
	return "Hello, " + name + "!"
}
