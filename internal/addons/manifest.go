package addons

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// ManifestName is the manifest's file name inside an add-on's folder.
const ManifestName = "addon.json"

// Limits of a manifest. The file is the add-on author's, read as untrusted
// text: anything past a limit refuses the whole add-on.
const (
	MaxManifestBytes = 64 << 10
	MaxNameRunes     = 60
	MaxDescRunes     = 300
	MaxCommandBytes  = 200
	maxVersionBytes  = 40
)

var (
	idExpr      = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)
	versionExpr = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
)

// Manifest is `<config>/sona/addons/<id>/addon.json`. Unknown fields refuse
// the manifest: a field this version doesn't know could be one that grants
// something it doesn't enforce.
type Manifest struct {
	// ID is the add-on's ID: lowercase letters, digits and hyphens, starting
	// with a letter, 2 to 40 characters, and the name of its folder.
	ID string `json:"id"`
	// Name is the add-on's name, shown wherever it contributes something.
	Name string `json:"name"`
	// Version is semver.
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	// Command is the executable to run: a path inside the folder with `/`
	// separators, relative, without `..`.
	Command string `json:"command"`
	// Permissions are what the add-on asks for; the user grants them when
	// they enable it.
	Permissions []string `json:"permissions"`
	// Contributions are what the add-on adds to Sona's actions, palette and
	// Settings (contributions.go); Strings are their labels by locale.
	Contributions Contributions `json:"contributions,omitempty"`
	Strings       Strings       `json:"strings,omitempty"`
}

// Problem codes: why an add-on is refused. The UI words each; Detail is for
// logs and never reaches the user.
const (
	ProblemUnreadable   = "unreadable" // the folder or manifest can't be read
	ProblemTooLarge     = "manifest_too_large"
	ProblemManifest     = "manifest_invalid" // not JSON, an unknown field, a wrong type
	ProblemID           = "bad_id"
	ProblemReservedID   = "reserved_id"
	ProblemIDMismatch   = "id_mismatch" // the ID is not the folder's name
	ProblemName         = "bad_name"
	ProblemVersion      = "bad_version"
	ProblemCommand      = "bad_command" // not a plain path inside the folder
	ProblemPermission   = "bad_permission"
	ProblemNoExecutable = "no_executable"      // missing, not a regular file or not executable
	ProblemEscapes      = "executable_escapes" // a link leads out of the folder
	ProblemUnsafe       = "unsafe_owner"       // another user's, or writable by others
	ProblemTooMany      = "too_many_addons"
	ProblemContribution = "bad_contribution"
	ProblemStrings      = "bad_strings"
)

// Problem is why an add-on is refused.
type Problem struct {
	Code   string
	Detail string
}

func (p *Problem) Error() string {
	if p.Detail == "" {
		return "addon: " + p.Code
	}
	return "addon: " + p.Code + ": " + p.Detail
}

func problem(code, format string, args ...any) *Problem {
	return &Problem{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// ProblemOf is err's problem code, ProblemUnreadable for any other error.
func ProblemOf(err error) string {
	var p *Problem
	if errors.As(err, &p) {
		return p.Code
	}
	return ProblemUnreadable
}

// reservedIDs belong to Sona's own bundled add-ons (#302): a folder in the
// user's directory may not claim them.
var reservedIDs = []string{"lastfm", "listenbrainz", "discord"}

// Reserved reports whether id is one only Sona's bundle may use.
func Reserved(id string) bool {
	return strings.HasPrefix(id, "sona") || slices.Contains(reservedIDs, id)
}

// ParseManifest reads and validates a manifest. It checks the text only:
// the folder it sits in, its ID against that folder and the executable are
// the scanner's (Scan).
func ParseManifest(data []byte) (Manifest, error) {
	if len(data) > MaxManifestBytes {
		return Manifest{}, problem(ProblemTooLarge, "%d bytes", len(data))
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, problem(ProblemManifest, "%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, problem(ProblemManifest, "text after the manifest")
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m *Manifest) validate() error {
	if !idExpr.MatchString(m.ID) {
		return problem(ProblemID, "%q", m.ID)
	}
	if err := plainText(m.Name, 1, MaxNameRunes); err != nil {
		return problem(ProblemName, "name: %v", err)
	}
	if m.Description != "" {
		if err := plainText(m.Description, 0, MaxDescRunes); err != nil {
			return problem(ProblemName, "description: %v", err)
		}
	}
	if len(m.Version) > maxVersionBytes || !versionExpr.MatchString(m.Version) {
		return problem(ProblemVersion, "%q", m.Version)
	}
	if err := checkCommand(m.Command); err != nil {
		return err
	}
	if len(m.Permissions) > len(cli.Permissions) {
		return problem(ProblemPermission, "%d permissions", len(m.Permissions))
	}
	seen := map[string]bool{}
	for _, p := range m.Permissions {
		if !cli.KnownPermission(p) || seen[p] {
			return problem(ProblemPermission, "%q", p)
		}
		seen[p] = true
	}
	if err := m.Contributions.validate(); err != nil {
		return err
	}
	return m.Strings.validate(m.Contributions)
}

// plainText accepts text of min to max runes without control characters,
// bidirectional controls or other format characters, with no edge spaces:
// what a user is shown must read as it is stored.
func plainText(s string, min, max int) error {
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < min || n > max {
		return fmt.Errorf("not %d to %d characters of text", min, max)
	}
	if s != strings.TrimSpace(s) {
		return errors.New("spaces at an edge")
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.In(r, unicode.Zl, unicode.Zp) {
			return fmt.Errorf("control or format character U+%04X", r)
		}
	}
	return nil
}

// checkCommand accepts a relative path with `/` separators that stays inside
// the folder: no `..`, no drive, no backslash, no empty or `.` element.
func checkCommand(c string) error {
	bad := func() error { return problem(ProblemCommand, "%q", c) }
	if c == "" || len(c) > MaxCommandBytes || !utf8.ValidString(c) {
		return bad()
	}
	if strings.ContainsAny(c, "\\:\x00") || strings.HasPrefix(c, "/") {
		return bad()
	}
	for _, r := range c {
		if unicode.IsControl(r) {
			return bad()
		}
	}
	for el := range strings.SplitSeq(c, "/") {
		if el == "" || el == "." || el == ".." {
			return bad()
		}
	}
	return nil
}
