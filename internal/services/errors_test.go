package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/motion"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
	"github.com/coredev-uk/sona-go/internal/scrobble"
	"github.com/coredev-uk/sona-go/internal/update"
)

// codeConstants reads every ErrorCode constant declared in errors.go,
// by name, so a new code can't skip the tests below.
func codeConstants(t *testing.T) map[string]ErrorCode {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]ErrorCode{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "ErrorCode" {
				continue
			}
			for i, n := range vs.Names {
				v, err := strconv.Unquote(vs.Values[i].(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				out[n.Name] = ErrorCode(v)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no ErrorCode constants in errors.go")
	}
	return out
}

// TestEveryCodeIsClassified checks that the codes are unique and that
// CodeOf gives each one for an error that should carry it.
func TestEveryCodeIsClassified(t *testing.T) {
	examples := map[ErrorCode]error{
		CodeInvalidCredentials:  appleid.ErrInvalidCredentials,
		CodeIncorrectCode:       appleid.ErrIncorrectCode,
		CodeLocked:              appleid.ErrLocked,
		CodeActionRequired:      appleid.ErrActionRequired,
		CodeNoSubscription:      appleid.ErrNoSubscription,
		CodeUnsupportedCountry:  appleid.ErrUnsupportedCountry,
		CodeUnsupported2FA:      appleid.ErrUnsupported2FA,
		CodeOutOfOrder:          appleid.ErrOutOfOrder,
		CodeTimeout:             mut.ErrTimeout,
		CodeCancelled:           mut.ErrCanceled,
		CodeNetwork:             &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("no route")},
		CodeMUTRejected:         auth.ErrMUTRejected,
		CodeDevTokenRejected:    auth.ErrDevTokenRejected,
		CodeNotSignedIn:         auth.ErrNotSignedIn,
		CodeUnauthorized:        applemusic.ErrUnauthorized,
		CodeNotFound:            library.ErrNotFound,
		CodeNotPlayable:         applemusic.ErrNotPlayable,
		CodeLiveStation:         coordinator.ErrLiveStation,
		CodeStation:             coordinator.ErrStation,
		CodeNotReleased:         ErrNotReleased,
		CodeSyncRunning:         library.ErrSyncRunning,
		CodeInvalidArgument:     ErrInvalidArgument,
		CodeNameRequired:        ErrNameRequired,
		CodeFolderNotEmpty:      ErrFolderNotEmpty,
		CodeNotAppleMusicLink:   ErrNotAppleMusicLink,
		CodeClipboard:           ErrClipboard,
		CodeTokenRequired:       ErrTokenRequired,
		CodeBrowserOpen:         ErrOpenBrowser,
		CodeSignOutIncomplete:   auth.ErrSignOutIncomplete,
		CodeStoredUnreadable:    auth.ErrStoredUnreadable,
		CodeSettingsNotExport:   config.ErrNotExport,
		CodeSettingsNewerExport: config.ErrNewerExport,
		CodeSettingsBadValue:    config.ErrBadValue,
		CodeSettingsRead:        ErrSettingsRead,
		CodeSettingsWrite:       ErrSettingsWrite,
		CodeNoRendition:         motion.ErrNoRendition,
		CodeConflict:            ErrConflict,
		CodePinsFull:            applemusic.ErrPinsFull,
		CodeNotConfigured:       scrobble.ErrNotConfigured,
		CodePrivateForced:       privatemode.ErrForced,
		CodeUnsupported:         errors.ErrUnsupported,
		CodeNoCDM:               playback.ErrNoCDM,
		CodeUpdateSignature:     update.ErrSignature,
		CodeUpdateFailed:        update.ErrInstall,
		CodeKeyring:             auth.ErrKeyring,
		CodeVideoHDOnly:         playback.ErrVideoHDOnly,
		CodeVideoLicence:        playback.ErrVideoLicence,
		CodeUnknown:             errors.New("boom"),
	}
	seen := map[ErrorCode]string{}
	for name, code := range codeConstants(t) {
		if code == "" {
			t.Errorf("%s is empty, which means no error", name)
		}
		if other, ok := seen[code]; ok {
			t.Errorf("%s and %s are both %q", name, other, code)
		}
		seen[code] = name
		err, ok := examples[code]
		if !ok {
			t.Errorf("%s (%q) has no example error here", name, code)
			continue
		}
		if got := CodeOf(err); got != code {
			t.Errorf("CodeOf(%v) = %q, want %s (%q)", err, got, name, code)
		}
	}
	// Engines report the video failures wrapped.
	for err, code := range map[error]ErrorCode{
		fmt.Errorf("native: video 1440841363: %w", playback.ErrVideoHDOnly):                    CodeVideoHDOnly,
		fmt.Errorf("native: video licence: %w: %w", playback.ErrVideoLicence, errors.New("x")): CodeVideoLicence,
	} {
		if got := CodeOf(err); got != code {
			t.Errorf("CodeOf(%v) = %q, want %q", err, got, code)
		}
	}
}

// uiTextField matches the field names that would carry Go's English to the
// UI if they were plain strings.
var uiTextField = regexp.MustCompile(`(Error|Err|Reason|Detail|Note|Message|Last)$`)

// englishAllowed are plain string fields of that kind which may carry text,
// with the reason.
var englishAllowed = map[string]string{
	"CallError.Message":           "Go's error text beside the code, for the frontend's logs only",
	"DiagBundle.Text":             "the bug-report bundle, copied for the maintainers",
	"MotionReport.Last":           "the web view's text for the bug-report bundle",
	"AppleTranslations.Messages":  "Apple's own strings",
	"CallError.AppleMessage":      "Apple's own, localised sign-in message for a code Sona doesn't map",
	"AuthStatus.LastAppleMessage": "Apple's own, localised sign-in message for a code Sona doesn't map",
}

// TestNoGoTextReachesTheUI walks every bound method's parameters and
// results and every event payload, and fails on a plain string field named
// like an error or a note: the UI words codes, so such a field must be a
// code type (ErrorCode or its own enum).
func TestNoGoTextReachesTheUI(t *testing.T) {
	constructors := map[string]any{
		"auth": newAuthService, "catalog": newCatalogService, "library": newLibraryService,
		"playback": newPlaybackService, "cdm": newCDMService, "search": newSearchService,
		"discover": newDiscoverService, "playlists": newPlaylistsService, "radio": newRadioService,
		"system": newSystemService, "updates": newUpdatesService, "integrations": newIntegrationsService,
		"motion": newMotionService, "desktop": newDesktopService, "scrobble": newScrobbleService,
		"audio": newAudioService, "storage": newStorageService, "private": newPrivateService,
		"platform": newPlatformService, "reminders": newRemindersService, "translations": newTranslationsService,
		"lyrics": newLyricsService, "concerts": newConcertsService,
	}
	var roots []reflect.Type
	for _, f := range features {
		c, ok := constructors[f.name]
		if !ok {
			t.Errorf("feature %q: add its constructor to this test", f.name)
			continue
		}
		svc := reflect.TypeOf(c).Out(0)
		for i := range svc.NumMethod() {
			m := svc.Method(i).Type
			for j := 1; j < m.NumIn(); j++ {
				roots = append(roots, m.In(j))
			}
			for j := range m.NumOut() {
				roots = append(roots, m.Out(j))
			}
		}
	}
	roots = append(roots, reflect.TypeFor[CallError]())
	roots = append(roots, eventPayloads(t)...)

	str := reflect.TypeFor[string]()
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(ty reflect.Type) {
		if seen[ty] {
			return
		}
		seen[ty] = true
		switch ty.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			walk(ty.Elem())
		case reflect.Map:
			walk(ty.Key())
			walk(ty.Elem())
		case reflect.Struct:
			for i := range ty.NumField() {
				f := ty.Field(i)
				if !f.IsExported() || f.Tag.Get("json") == "-" {
					continue
				}
				key := ty.Name() + "." + f.Name
				if f.Type == str && uiTextField.MatchString(f.Name) {
					if _, ok := englishAllowed[key]; !ok {
						t.Errorf("%s.%s is a plain string: send a code type the frontend words", ty.PkgPath(), key)
					}
				}
				walk(f.Type)
			}
		}
	}
	for _, r := range roots {
		walk(r)
	}
}

// eventPayloads are the payload types of every event the package declares:
// each Event… constant must be listed here with its payload type.
func eventPayloads(t *testing.T) []reflect.Type {
	t.Helper()
	known := map[string]reflect.Type{
		"EventAuthChanged":        reflect.TypeFor[AuthStatus](),
		"EventCDMStatus":          reflect.TypeFor[CDMStatus](),
		"EventHome":               reflect.TypeFor[HomePage](),
		"EventAppOpen":            reflect.TypeFor[OpenTarget](),
		"EventIntegrationsStatus": reflect.TypeFor[IntegrationsStatus](),
		"EventSyncProgress":       reflect.TypeFor[SyncProgress](),
		"EventSyncDone":           reflect.TypeFor[SyncDone](),
		"EventItemState":          reflect.TypeFor[ItemState](),
		"EventPower":              reflect.TypeFor[PowerState](),
		"EventPlaybackState":      reflect.TypeFor[PlaybackState](),
		"EventPlaybackQueue":      reflect.TypeFor[QueueState](),
		"EventPlaylistsChanged":   reflect.TypeFor[PlaylistsChanged](),
		"EventPrivateChanged":     reflect.TypeFor[PrivateState](),
		"EventRemindersChanged":   reflect.TypeFor[[]Reminder](),
		"EventScrobbleChanged":    reflect.TypeFor[ScrobbleStatus](),
		"EventSettingsChanged":    reflect.TypeFor[config.Settings](),
		"EventAppleTranslations":  reflect.TypeFor[AppleTranslations](),
		"EventAppleLanguages":     reflect.TypeFor[AppleLanguages](),
		"EventUpdateAvailable":    reflect.TypeFor[UpdateStatus](),
		"EventUpdateInstall":      reflect.TypeFor[UpdateStatus](),
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []reflect.Type
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				for _, id := range sp.(*ast.ValueSpec).Names {
					if !strings.HasPrefix(id.Name, "Event") {
						continue
					}
					ty, ok := known[id.Name]
					if !ok {
						t.Errorf("event %s: add its payload to eventPayloads", id.Name)
						continue
					}
					out = append(out, ty)
				}
			}
		}
	}
	if len(out) < len(known) {
		t.Errorf("found %d event payloads, want at least %d", len(out), len(known))
	}
	return slices.Clip(out)
}

// TestAppleMessage: a sign-in failure Apple worded with a code Sona doesn't
// map keeps Apple's (localised) text as data, in a rejected call and in
// AuthStatus; mapped codes and Sona's own text carry none.
func TestAppleMessage(t *testing.T) {
	const french = "Ce compte Apple est désactivé."
	unmapped := fmt.Errorf("sign in: %w", appleid.ParseServiceError(400, []byte(`{"serviceErrors":[{"code":"-99999","message":"`+french+`"}]}`)))
	mapped := appleid.ParseServiceError(401, []byte(`{"serviceErrors":[{"code":"-20101","message":"Mot de passe incorrect."}]}`))
	ours := &appleid.ServiceError{Status: 500, Message: "Apple Music token request failed"}
	if got := AppleMessage(unmapped); got != french {
		t.Errorf("unmapped: %q, want Apple's text", got)
	}
	for name, err := range map[string]error{"mapped": mapped, "Sona's text": ours, "other": errors.New("x")} {
		if got := AppleMessage(err); got != "" {
			t.Errorf("%s: %q, want none", name, got)
		}
	}
	var ce CallError
	if err := json.Unmarshal(MarshalError(unmapped), &ce); err != nil || ce.Code != CodeUnknown || ce.AppleMessage != french {
		t.Errorf("MarshalError = %+v, %v", ce, err)
	}
	st := (&AuthService{}).status(auth.Status{State: auth.StateSignedOut, LastError: "x", LastCause: unmapped})
	if st.LastErrorCode != CodeUnknown || st.LastAppleMessage != french {
		t.Errorf("AuthStatus = %+v", st)
	}
}
