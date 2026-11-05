//go:build probe && !windows

package main

import _ "github.com/coredev-uk/sona-go/internal/memprobe" // SIGUSR1 writes the Go heap profile (cmd/memprobe)
