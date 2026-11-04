package main

import (
	"reflect"
	"testing"
)

func TestCollide(t *testing.T) {
	got := collide([]string{"a/B.go", "a/b.go", "A/c.go", "d/e.go"})
	want := [][]string{{"A", "a"}, {"a/B.go", "a/b.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("collide = %v, want %v", got, want)
	}
	if got := collide([]string{"a/b.go", "a/c.go"}); len(got) != 0 {
		t.Fatalf("collide = %v, want none", got)
	}
}
