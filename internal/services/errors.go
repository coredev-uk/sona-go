package services

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// Error codes carried to the frontend in a rejected call's err.cause.code
// (see MarshalError) and in AuthStatus.LastErrorCode.
const (
	CodeInvalidCredentials = "invalid_credentials"
	CodeIncorrectCode      = "incorrect_code"
	CodeLocked             = "locked"
	CodeActionRequired     = "action_required"
	CodeNoSubscription     = "no_subscription"
	CodeUnsupportedCountry = "unsupported_country"
	CodeUnsupported2FA     = "unsupported_2fa"
	CodeOutOfOrder         = "out_of_order"
	CodeTimeout            = "timeout"
	CodeCancelled          = "cancelled"
	CodeNetwork            = "network"
	CodeMUTRejected        = "mut_rejected"
	CodeDevTokenRejected   = "dev_token_rejected"
	CodeNotSignedIn        = "not_signed_in"
	CodeUnauthorized       = "unauthorized" // Apple refused the request (401/403); auth triage runs
	CodeNotFound           = "not_found"
	CodeNotPlayable        = "not_playable"
	CodeNotReleased        = "not_released" // the song isn't out yet (ErrNotReleased)
	CodeSyncRunning        = "sync_running"
	CodeInvalidArgument    = "invalid_argument"
	CodeConflict           = "conflict"       // the item changed elsewhere since it was read
	CodePinsFull           = "pins_full"      // Apple keeps at most six pins
	CodeNotConfigured      = "not_configured" // this build lacks the service's API account (Last.fm)
	CodePrivateForced      = "private_forced" // SONA_PRIVATE=1 keeps private mode on
	CodeUnsupported        = "unsupported"    // this platform can't do it (Capabilities says so)
	CodeNoCDM              = "no_cdm"         // the Widevine CDM isn't ready yet (CDMService says why)
	CodeKeyring            = "keyring"        // the token store failed or didn't answer in time
	CodeUnknown            = "unknown"
)

// ErrInvalidArgument wraps argument validation failures.
var ErrInvalidArgument = errors.New("invalid argument")

// ErrorCode classifies err into one of the Code constants.
func ErrorCode(err error) string {
	var (
		ue *url.Error
		ne net.Error
	)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, appleid.ErrInvalidCredentials):
		return CodeInvalidCredentials
	case errors.Is(err, appleid.ErrIncorrectCode):
		return CodeIncorrectCode
	case errors.Is(err, appleid.ErrLocked):
		return CodeLocked
	case errors.Is(err, appleid.ErrActionRequired):
		return CodeActionRequired
	case errors.Is(err, appleid.ErrNoSubscription):
		return CodeNoSubscription
	case errors.Is(err, appleid.ErrUnsupportedCountry):
		return CodeUnsupportedCountry
	case errors.Is(err, appleid.ErrUnsupported2FA):
		return CodeUnsupported2FA
	case errors.Is(err, appleid.ErrOutOfOrder):
		return CodeOutOfOrder
	case errors.Is(err, auth.ErrKeyring):
		return CodeKeyring
	case errors.Is(err, mut.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return CodeTimeout
	case errors.Is(err, mut.ErrCanceled), errors.Is(err, context.Canceled):
		return CodeCancelled
	case errors.Is(err, auth.ErrMUTRejected):
		return CodeMUTRejected
	case errors.Is(err, auth.ErrDevTokenRejected):
		return CodeDevTokenRejected
	case errors.Is(err, auth.ErrNotSignedIn):
		return CodeNotSignedIn
	case errors.Is(err, applemusic.ErrNotPlayable):
		return CodeNotPlayable
	case errors.Is(err, ErrNotReleased):
		return CodeNotReleased
	case errors.Is(err, applemusic.ErrUnauthorized):
		return CodeUnauthorized
	case errors.Is(err, applemusic.ErrNotFound), errors.Is(err, library.ErrNotFound), errors.Is(err, catalog.ErrNoStorefront),
		errors.Is(err, lyrics.ErrNotFound):
		return CodeNotFound
	case errors.Is(err, library.ErrSyncRunning):
		return CodeSyncRunning
	case errors.Is(err, ErrConflict):
		return CodeConflict
	case errors.Is(err, applemusic.ErrPinsFull):
		return CodePinsFull
	case errors.Is(err, scrobble.ErrNotConfigured):
		return CodeNotConfigured
	case errors.Is(err, scrobble.ErrAuth):
		return CodeInvalidCredentials
	case errors.Is(err, scrobble.ErrExpired):
		return CodeTimeout
	case errors.Is(err, ErrInvalidArgument):
		return CodeInvalidArgument
	case errors.Is(err, privatemode.ErrForced):
		return CodePrivateForced
	case errors.Is(err, playback.ErrNoCDM):
		return CodeNoCDM
	case errors.Is(err, errors.ErrUnsupported):
		return CodeUnsupported
	case errors.As(err, &ue), errors.As(err, &ne):
		return CodeNetwork
	}
	return CodeUnknown
}

// CallError is the JSON a rejected binding call carries as err.cause.
type CallError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// MarshalError is the application's Options.MarshalError: every service
// error reaches JS as err.cause = {code, message}.
func MarshalError(err error) []byte {
	b, _ := json.Marshal(CallError{Code: ErrorCode(err), Message: err.Error()})
	return b
}
