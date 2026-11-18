//go:build !dev

package own

import "log/slog"

// devEndpoints is none in a release build: the add-ons only ever talk to the
// real services.
func devEndpoints(*slog.Logger) Endpoints { return Endpoints{} }
