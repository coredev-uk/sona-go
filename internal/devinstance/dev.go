//go:build dev

package devinstance

import "os"

// Name is the sanitised SONA_DEV_INSTANCE, "" when unset.
func Name() string { return sanitize(os.Getenv(Env)) }
