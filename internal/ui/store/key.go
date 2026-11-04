package store

import (
	"fmt"
	"reflect"
	"strings"
)

// Key names a query: ("library", "albums", opts). It mirrors the TanStack
// keys of v0.2.0 (["library","albums",…]), so Invalidate(Key{"library"})
// refreshes every query below it. Elements should be comparable values
// (strings, numbers, bools, structs of them), never pointers.
type Key []any

// hasPrefix reports whether k starts with p, element by element.
func (k Key) hasPrefix(p Key) bool {
	if len(p) > len(k) {
		return false
	}
	for i := range p {
		if !sameElem(k[i], p[i]) {
			return false
		}
	}
	return true
}

func sameElem(a, b any) bool {
	if ta, tb := reflect.TypeOf(a), reflect.TypeOf(b); ta != tb {
		return false
	} else if ta == nil || ta.Comparable() {
		return safeEqual(a, b)
	}
	return reflect.DeepEqual(a, b)
}

// safeEqual is == for a comparable type that may still panic (an interface
// field holding a slice).
func safeEqual(a, b any) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = reflect.DeepEqual(a, b)
		}
	}()
	return a == b
}

// id is the key as a map key: the elements' types and values.
func (k Key) id() string {
	var b strings.Builder
	for _, e := range k {
		fmt.Fprintf(&b, "%T:%#v\x00", e, e)
	}
	return b.String()
}

func (k Key) String() string { return fmt.Sprint([]any(k)) }
