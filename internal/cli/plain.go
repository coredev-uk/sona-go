package cli

import "reflect"

// plain returns a copy of v in which every string is made safe for a
// terminal ([TerminalSafe]): escape sequences are gone, other control
// characters, line breaks and bidirectional controls are a space, invalid
// UTF-8 is U+FFFD. It is how the text commands print what Apple, a station,
// LRCLIB or a library upload supplied (titles, lyrics, errors): a terminal
// obeys those sequences (a window title, a fake screen, OSC 52 clipboard
// writes), so nothing third-party reaches one unprocessed. It walks structs,
// pointers, slices, maps and interfaces, so a field added to a result is
// covered without another call. v itself is not changed.
//
// JSON output does not use it: JSON escapes control characters, and its
// values are the data as received.
func plain[T any](v T) T {
	out := reflect.New(reflect.TypeFor[T]()).Elem()
	plainValue(out, reflect.ValueOf(&v).Elem())
	return out.Interface().(T)
}

func plainValue(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.String:
		dst.SetString(TerminalSafe(src.String()))
	case reflect.Pointer:
		if !src.IsNil() {
			dst.Set(reflect.New(src.Type().Elem()))
			plainValue(dst.Elem(), src.Elem())
		}
	case reflect.Interface:
		if !src.IsNil() {
			inner := reflect.New(src.Elem().Type()).Elem()
			plainValue(inner, src.Elem())
			dst.Set(inner)
		}
	case reflect.Struct:
		dst.Set(src) // scalars and unexported fields as they are
		for i := range src.NumField() {
			if dst.Field(i).CanSet() {
				plainValue(dst.Field(i), src.Field(i))
			}
		}
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeSlice(src.Type(), src.Len(), src.Len()))
		for i := range src.Len() {
			plainValue(dst.Index(i), src.Index(i))
		}
	case reflect.Array:
		for i := range src.Len() {
			plainValue(dst.Index(i), src.Index(i))
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		dst.Set(reflect.MakeMapWithSize(src.Type(), src.Len()))
		for it := src.MapRange(); it.Next(); {
			v := reflect.New(src.Type().Elem()).Elem()
			plainValue(v, it.Value())
			dst.SetMapIndex(it.Key(), v)
		}
	default:
		dst.Set(src)
	}
}
