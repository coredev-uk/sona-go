package services

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// systemInfo is the diagnostics bundle's "System" section for this OS.
func systemInfo() map[string]string {
	return systemSection(runtime.GOOS, osName(), os.Getenv)
}

// systemSection holds the OS name on every system. The desktop and the
// session type (XDG_CURRENT_DESKTOP, XDG_SESSION_TYPE) are only added where
// desktops follow XDG; macOS and Windows have one desktop and no such
// variables, so the keys would always be empty there.
func systemSection(goos, osName string, getenv func(string) string) map[string]string {
	s := map[string]string{"os": osName}
	if goos != "darwin" && goos != "windows" {
		s["desktop"] = getenv("XDG_CURRENT_DESKTOP")
		s["session"] = getenv("XDG_SESSION_TYPE")
	}
	return s
}

// osReleaseName is an os-release file's PRETTY_NAME ("" if it has none).
func osReleaseName(r io.Reader) string {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// macOSName formats sysctl's kern.osproductversion ("27.2") and
// kern.osversion (the build, "26B123") as "macOS 27.2 (26B123)", leaving
// out whichever is unknown.
func macOSName(version, build string) string {
	name := "macOS"
	if version != "" {
		name += " " + version
	}
	if build != "" {
		name += " (" + build + ")"
	}
	return name
}

// windowsName formats the registry's Windows NT CurrentVersion values as
// "Windows 11 Pro 24H2 (build 26100.2605)", leaving out whichever is
// unknown. ProductName still says "Windows 10" on Windows 11, so a build
// from 22000 (Windows 11's first) on corrects it.
func windowsName(product, displayVersion, build string, ubr uint64) string {
	if product == "" {
		product = "Windows"
	}
	if n, err := strconv.Atoi(build); err == nil && n >= 22000 {
		if rest, ok := strings.CutPrefix(product, "Windows 10"); ok {
			product = "Windows 11" + rest
		}
	}
	name := product
	if displayVersion != "" {
		name += " " + displayVersion
	}
	if build != "" {
		name += " (build " + build
		if ubr > 0 {
			name += "." + strconv.FormatUint(ubr, 10)
		}
		name += ")"
	}
	return name
}
