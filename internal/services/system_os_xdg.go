//go:build !darwin && !windows

package services

import "os"

// osName is /etc/os-release's PRETTY_NAME ("" if unknown).
func osName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	return osReleaseName(f)
}
