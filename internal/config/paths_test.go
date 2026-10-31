package config

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
