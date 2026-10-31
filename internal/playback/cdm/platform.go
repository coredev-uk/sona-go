package cdm

import (
	"fmt"
	"runtime"
)

// Platform is one OS and architecture Google builds the Widevine CDM for:
// where Mozilla's manifest lists it, where the library sits inside the
// CRX, and what the library must be.
type Platform struct {
	GOOS, GOARCH string
	// Mozilla is the platform's key in Mozilla's manifest (an alias is
	// followed).
	Mozilla string
	// Dir is the CRX's sub-package directory under _platform_specific/,
	// also the directory the library is installed in.
	Dir string
	// Lib is the library's file name.
	Lib string
	// format checks the library's binary header (binfmt.go).
	format binaryFormat
}

// String is "goos/goarch".
func (p Platform) String() string { return p.GOOS + "/" + p.GOARCH }

// platforms are the platforms Sona can install the CDM for. Mozilla lists
// each of them except Linux on arm64 (Firefox gets no Widevine there yet);
// a platform the manifest lacks fails with CodeUnsupportedPlatform.
var platforms = []Platform{
	{GOOS: "linux", GOARCH: "amd64", Mozilla: "Linux_x86_64-gcc3", Dir: "linux_x64", Lib: "libwidevinecdm.so", format: elfX86_64},
	{GOOS: "linux", GOARCH: "arm64", Mozilla: "Linux_aarch64-gcc3", Dir: "linux_arm64", Lib: "libwidevinecdm.so", format: elfAArch64},
	{GOOS: "darwin", GOARCH: "arm64", Mozilla: "Darwin_aarch64-gcc3", Dir: "mac_arm64", Lib: "libwidevinecdm.dylib", format: machoARM64},
	{GOOS: "darwin", GOARCH: "amd64", Mozilla: "Darwin_x86_64-gcc3", Dir: "mac_x64", Lib: "libwidevinecdm.dylib", format: machoX86_64},
	{GOOS: "windows", GOARCH: "amd64", Mozilla: "WINNT_x86_64-msvc", Dir: "win_x64", Lib: "widevinecdm.dll", format: peAMD64},
	{GOOS: "windows", GOARCH: "arm64", Mozilla: "WINNT_aarch64-msvc-aarch64", Dir: "win_arm64", Lib: "widevinecdm.dll", format: peARM64},
}

// PlatformFor returns the platform for goos/goarch.
func PlatformFor(goos, goarch string) (Platform, error) {
	for _, p := range platforms {
		if p.GOOS == goos && p.GOARCH == goarch {
			return p, nil
		}
	}
	return Platform{}, &Error{Code: CodeUnsupportedPlatform, Err: fmt.Errorf("no Widevine CDM for %s/%s", goos, goarch)}
}

// Current is the platform Sona runs on.
func Current() (Platform, error) { return PlatformFor(runtime.GOOS, runtime.GOARCH) }
