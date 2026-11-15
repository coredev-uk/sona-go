//go:build dev

package app

import "github.com/coredev-uk/sona-go/internal/ui/features/inspect"

// A development build adds the item inspector (right-click → Inspect).
func init() { factories = append(factories, inspect.New) }
