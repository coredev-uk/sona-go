package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/scrobble"
)

// ErrorCode is a stable error code the frontend words through its own
// catalogue (frontend/src/shared/lib/errors.ts): a rejected call carries
// one in err.cause.code (see MarshalError), and DTOs carry one where Go
// would otherwise send English (AuthStatus.LastErrorCode, sync events…).
// Wails binds it as the TypeScript enum ErrorCode, so the frontend can't
// miss a code. Go's error text never reaches the UI; it is for logs.
type ErrorCode string

// The error codes. TestEveryCodeIsClassified needs an example error for
// each one.
const (
	CodeInvalidCredentials  ErrorCode = "invalid_credentials"
	CodeIncorrectCode       ErrorCode = "incorrect_code"
	CodeLocked              ErrorCode = "locked"
	CodeActionRequired      ErrorCode = "action_required"
	CodeNoSubscription      ErrorCode = "no_subscription"
	CodeUnsupportedCountry  ErrorCode = "unsupported_country"
	CodeUnsupported2FA      ErrorCode = "unsupported_2fa"
	CodeOutOfOrder          ErrorCode = "out_of_order"
	CodeTimeout             ErrorCode = "timeout"
	CodeCancelled           ErrorCode = "cancelled"
	CodeNetwork             ErrorCode = "network"
	CodeMUTRejected         ErrorCode = "mut_rejected"
	CodeDevTokenRejected    ErrorCode = "dev_token_rejected"
	CodeNotSignedIn         ErrorCode = "not_signed_in"
	CodeUnauthorized        ErrorCode = "unauthorized" // Apple refused the request (401/403); auth triage runs
	CodeNotFound            ErrorCode = "not_found"
	CodeNotPlayable         ErrorCode = "not_playable"
	CodeNotReleased         ErrorCode = "not_released" // the song isn't out yet (ErrNotReleased)
	CodeSyncRunning         ErrorCode = "sync_running"
	CodeInvalidArgument     ErrorCode = "invalid_argument"
	CodeNameRequired        ErrorCode = "name_required"         // a playlist or folder needs a name (ErrNameRequired)
	CodeFolderNotEmpty      ErrorCode = "folder_not_empty"      // ErrFolderNotEmpty
	CodeNotAppleMusicLink   ErrorCode = "not_apple_music_link"  // ErrNotAppleMusicLink
	CodeClipboard           ErrorCode = "clipboard_unavailable" // ErrClipboard
	CodeTokenRequired       ErrorCode = "token_required"        // a pasted token is empty (ErrTokenRequired)
	CodeBrowserOpen         ErrorCode = "browser_open"          // the web browser couldn't be opened
	CodeSignOutIncomplete   ErrorCode = "sign_out_incomplete"   // auth.ErrSignOutIncomplete
	CodeStoredUnreadable    ErrorCode = "stored_unreadable"     // stored sign-in discarded (auth.ErrStoredUnreadable)
	CodeSettingsNotExport   ErrorCode = "settings_not_export"   // config.ErrNotExport
	CodeSettingsNewerExport ErrorCode = "settings_newer_export" // config.ErrNewerExport
	CodeSettingsBadValue    ErrorCode = "settings_bad_value"    // config.ErrBadValue
	CodeSettingsRead        ErrorCode = "settings_read"         // ErrSettingsRead
	CodeSettingsWrite       ErrorCode = "settings_write"        // ErrSettingsWrite
	CodeNoRendition         ErrorCode = "no_rendition"          // Apple has no motion video in the codec (motion.ErrNoRendition)
	CodeConflict            ErrorCode = "conflict"              // the item changed elsewhere since it was read
	CodePinsFull            ErrorCode = "pins_full"             // Apple keeps at most six pins
	CodeNotConfigured       ErrorCode = "not_configured"        // this build lacks the service's API account (Last.fm)
	CodePrivateForced       ErrorCode = "private_forced"        // SONA_PRIVATE=1 keeps private mode on
	CodeUnsupported         ErrorCode = "unsupported"           // this platform can't do it (Capabilities says so)
	CodeNoCDM               ErrorCode = "no_cdm"                // the Widevine CDM isn't ready yet (CDMService says why)
	CodeKeyring             ErrorCode = "keyring"               // the token store failed or didn't answer in time
	CodeUnknown             ErrorCode = "unknown"               // none of the above; the frontend shows a generic message
)

// Errors of the services themselves that carry their own code.
var (
	// ErrInvalidArgument wraps argument validation failures.
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrNameRequired is a playlist or folder without a name; it wraps
	// ErrInvalidArgument.
	ErrNameRequired = fmt.Errorf("%w: a name is required", ErrInvalidArgument)
	// ErrNotAppleMusicLink is a link that isn't on music.apple.com; it
	// wraps ErrInvalidArgument.
	ErrNotAppleMusicLink = fmt.Errorf("%w: not an Apple Music link", ErrInvalidArgument)
	// ErrTokenRequired is an empty pasted token; it wraps
	// ErrInvalidArgument.
	ErrTokenRequired = fmt.Errorf("%w: a token is required", ErrInvalidArgument)
	// ErrClipboard is a clipboard that couldn't take the text.
	ErrClipboard = errors.New("the clipboard is unavailable")
	// ErrOpenBrowser is a web page the system browser couldn't open.
	ErrOpenBrowser = errors.New("could not open a browser")
	// ErrSettingsRead and ErrSettingsWrite wrap I/O failures on a settings
	// export file.
	ErrSettingsRead  = errors.New("couldn't read the settings file")
	ErrSettingsWrite = errors.New("couldn't write the settings file")
)

// CodeOf classifies err into one of the Code constants; "" for nil.
func CodeOf(err error) ErrorCode {
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
	case errors.Is(err, auth.ErrSignOutIncomplete):
		return CodeSignOutIncomplete
	case errors.Is(err, auth.ErrStoredUnreadable):
		return CodeStoredUnreadable
	case errors.Is(err, auth.ErrBrowserOpen), errors.Is(err, ErrOpenBrowser):
		return CodeBrowserOpen
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
	case errors.Is(err, ErrFolderNotEmpty):
		return CodeFolderNotEmpty
	case errors.Is(err, ErrNameRequired):
		return CodeNameRequired
	case errors.Is(err, ErrNotAppleMusicLink):
		return CodeNotAppleMusicLink
	case errors.Is(err, ErrTokenRequired):
		return CodeTokenRequired
	case errors.Is(err, ErrInvalidArgument):
		return CodeInvalidArgument
	case errors.Is(err, ErrClipboard):
		return CodeClipboard
	case errors.Is(err, config.ErrNotExport):
		return CodeSettingsNotExport
	case errors.Is(err, config.ErrNewerExport):
		return CodeSettingsNewerExport
	case errors.Is(err, config.ErrBadValue):
		return CodeSettingsBadValue
	case errors.Is(err, ErrSettingsRead):
		return CodeSettingsRead
	case errors.Is(err, ErrSettingsWrite):
		return CodeSettingsWrite
	case errors.Is(err, motion.ErrNoRendition):
		return CodeNoRendition
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
// Message is Go's error text, for logs only: the UI words Code.
type CallError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	// AppleMessage is Apple's own wording of a sign-in failure Sona has no
	// code for (AppleMessage); Apple localised it, so the UI shows it.
	AppleMessage string `json:"appleMessage,omitempty"`
}

// AppleMessage is Apple's own, already localised wording of a failure
// that classifies as CodeUnknown: a sign-in error Apple sent with a code
// Sona doesn't map. "" for every other error, including Sona's own text.
func AppleMessage(err error) string {
	var se *appleid.ServiceError
	if CodeOf(err) != CodeUnknown || !errors.As(err, &se) {
		return ""
	}
	return se.AppleText()
}

// MarshalError is the application's Options.MarshalError: every service
// error reaches JS as err.cause = {code, message}.
func MarshalError(err error) []byte {
	b, _ := json.Marshal(CallError{Code: CodeOf(err), Message: err.Error(), AppleMessage: AppleMessage(err)})
	return b
}
