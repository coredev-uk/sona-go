package kit

// Entry is the value under key in the map *m, made (with the map) when
// missing: the per-key state of a page's shelves and lists.
func Entry[T any](m *map[string]*T, key string) *T {
	if *m == nil {
		*m = map[string]*T{}
	}
	v := (*m)[key]
	if v == nil {
		v = new(T)
		(*m)[key] = v
	}
	return v
}
