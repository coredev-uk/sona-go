package addons

import (
	"embed"
	"slices"
)

// Sona's own add-ons (Last.fm, ListenBrainz, Discord; #302) are registered
// here, in code, with manifests embedded in the binary. They are never read
// from the user's add-ons folder, whose copy of a reserved ID is refused
// (Reserved), and they are not a second program: the command of each is
// Sona's own executable with `--addon <id>`, so the release's signature
// covers them and no separate key exists. They run under the same
// supervisor, the same protocol and the same permission checks as any other
// add-on; what only they get is listed in docs/addons.md (Bundled add-ons).

//go:embed bundled/*.json
var bundledFS embed.FS

// bundledSecrets are the secret keys each bundled add-on may read with the
// secret method: the item `addon.<id>` of the keyring holds them by key.
// They are not settings rows (no page shows or edits them); Sona's own
// connection flows (Settings → Connections) write them.
var bundledSecrets = map[string][]string{
	"lastfm":       {"session"},
	"listenbrainz": {"token"},
	"discord":      nil,
}

// BundledIDs are the IDs of Sona's own add-ons, in the order the list shows
// them.
var BundledIDs = []string{"discord", "lastfm", "listenbrainz"}

// IsBundled reports whether id is one of Sona's own add-ons.
func IsBundled(id string) bool { return slices.Contains(BundledIDs, id) }

// BundledSecret reports whether key is one of the secrets bundled add-on id
// may read.
func BundledSecret(id, key string) bool { return slices.Contains(bundledSecrets[id], key) }

// BundledArgs are the arguments of Sona's own executable that run the
// bundled add-on id: the entry point cmd/sona dispatches before it starts
// anything else.
func BundledArgs(id string) []string { return []string{BundledFlag, id} }

// BundledFlag is the command-line flag that runs a bundled add-on:
// `sona --addon <id>`.
const BundledFlag = "--addon"

// BundledEntries are Sona's own add-ons as the scanner lists them, with
// exe, Sona's executable with every link resolved, as their program. An
// empty exe (Sona can't tell where it runs from) makes each entry refused
// rather than run something else.
func BundledEntries(exe string) []Entry {
	out := make([]Entry, 0, len(BundledIDs))
	for _, id := range BundledIDs {
		e := Entry{Dir: id, ID: id, Bundled: true}
		m, err := BundledManifest(id)
		if err != nil {
			e.Problem = asProblem(err)
			out = append(out, e)
			continue
		}
		e.Manifest = m
		if exe == "" {
			e.Problem = problem(ProblemNoExecutable, "Sona's own executable is unknown")
		} else {
			e.Exe, e.Args = exe, BundledArgs(id)
		}
		out = append(out, e)
	}
	return out
}

// BundledManifest reads the embedded manifest of id: the same checks as any
// manifest, and its ID is the one it is registered under. The command is
// there to satisfy the format; the supervisor runs Sona's executable.
func BundledManifest(id string) (Manifest, error) {
	data, err := bundledFS.ReadFile("bundled/" + id + ".json")
	if err != nil {
		return Manifest{}, problem(ProblemUnreadable, "%v", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return Manifest{}, err
	}
	if m.ID != id {
		return Manifest{}, problem(ProblemIDMismatch, "bundled %q holds %q", id, m.ID)
	}
	if !Reserved(m.ID) {
		return Manifest{}, problem(ProblemID, "bundled %q is not a reserved ID", m.ID)
	}
	return m, nil
}
