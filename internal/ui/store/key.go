package store

import (
	"fmt"
	"reflect"
	"strconv"
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
func (k Key) id() string { return string(k.appendID(nil)) }

// appendID appends id to dst. The common element types (strings, ints and
// bools, nearly every key) are written without fmt, in the form fmt's
// "%T:%#v" gives (an unsigned integer in decimal, not hex, which no other
// path produces); Use runs it every frame, so a caller with a stack buffer
// looks an entry up without allocating.
func (k Key) appendID(dst []byte) []byte {
	for _, e := range k {
		switch v := e.(type) {
		case string:
			dst = append(dst, "string:"...)
			dst = strconv.AppendQuote(dst, v)
		case int:
			dst = append(dst, "int:"...)
			dst = strconv.AppendInt(dst, int64(v), 10)
		case int64:
			dst = append(dst, "int64:"...)
			dst = strconv.AppendInt(dst, v, 10)
		case uint64:
			dst = append(dst, "uint64:"...)
			dst = strconv.AppendUint(dst, v, 10)
		case bool:
			dst = append(dst, "bool:"...)
			dst = strconv.AppendBool(dst, v)
		default:
			dst = fmt.Appendf(dst, "%T:%#v", e, e)
		}
		dst = append(dst, 0)
	}
	return dst
}

func (k Key) String() string { return fmt.Sprint([]any(k)) }
