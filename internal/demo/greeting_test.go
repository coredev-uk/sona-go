package demo

import "testing"

func TestGreeting(t *testing.T) {
	cases := map[string]string{
		"Sona":   "Hello, Sona!",
		"  Ada ": "Hello, Ada!",
		"":       "Hello, listener!",
		"   ":    "Hello, listener!",
	}
	for in, want := range cases {
		if got := Greeting(in); got != want {
			t.Errorf("Greeting(%q) = %q, want %q", in, got, want)
		}
	}
}
