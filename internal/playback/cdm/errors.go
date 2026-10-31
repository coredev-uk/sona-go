package cdm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Failure codes, in Error.Code and Status.Code: stable strings the
// frontend words.
const (
	// CodeNetwork: Mozilla's manifest or Google's package could not be
	// fetched (offline, DNS, TLS, an HTTP error, a stalled download).
	CodeNetwork = "network"
	// CodeUnsupportedPlatform: Google ships no Widevine CDM for this OS
	// and architecture in Mozilla's manifest.
	CodeUnsupportedPlatform = "unsupported_platform"
	// CodeManifest: the manifest is malformed or lists something Sona
	// refuses (another hash function, a bad version or hash).
	CodeManifest = "manifest_invalid"
	// CodeUntrustedURL: the manifest points the package somewhere other
	// than Google's download hosts over HTTPS.
	CodeUntrustedURL = "untrusted_url"
	// CodeTooLarge: the package or a file in it is larger than allowed,
	// or larger than the manifest says.
	CodeTooLarge = "too_large"
	// CodeHashMismatch: the package's SHA-512 or size is not the
	// manifest's.
	CodeHashMismatch = "hash_mismatch"
	// CodeSignature: the CRX3 signatures do not prove Google's Widevine
	// component.
	CodeSignature = "signature_invalid"
	// CodeArchive: the package's archive is malformed, unsafe (paths
	// leaving it), or lacks the library or its manifest, or the library
	// is not built for this platform.
	CodeArchive = "archive_invalid"
	// CodeLoad: the new CDM failed to load in the sandboxed host.
	CodeLoad = "load_failed"
	// CodeDisk: the install directory could not be written.
	CodeDisk = "disk"
	// CodeOverrideMissing: SONA_WIDEVINE_CDM names no library.
	CodeOverrideMissing = "override_missing"
)

// Error is a failure with its code.
type Error struct {
	Code string
	Err  error
}

func (e *Error) Error() string { return "cdm: " + e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

func fail(code string, format string, args ...any) *Error {
	return &Error{Code: code, Err: fmt.Errorf(format, args...)}
}

// CodeOf returns err's failure code: an Error's, CodeNetwork for network
// and timeout errors, else CodeDisk (local I/O is all that remains).
func CodeOf(err error) string {
	var (
		e  *Error
		ue *url.Error
		ne net.Error
	)
	switch {
	case err == nil:
		return ""
	case errors.As(err, &e):
		return e.Code
	case errors.As(err, &ue), errors.As(err, &ne), errors.Is(err, context.DeadlineExceeded):
		return CodeNetwork
	}
	return CodeDisk
}

// Errors from Manager.Library; both wrap playback.ErrNoCDM.
var (
	// ErrDownloading: the CDM is being downloaded and none is installed
	// yet.
	ErrDownloading = fmt.Errorf("cdm: the Widevine CDM is still downloading: %w", playback.ErrNoCDM)
	// ErrUnavailable: no CDM is installed and none is being downloaded
	// (the download has not started or failed: Status says why).
	ErrUnavailable = fmt.Errorf("cdm: no Widevine CDM is installed: %w", playback.ErrNoCDM)
)
