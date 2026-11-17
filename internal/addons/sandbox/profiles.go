package sandbox

import (
	"os"
	"regexp"
	"slices"
)

// profilePrefix starts the name of an add-on's AppContainer profile on
// Windows: "sona.addon.<id>" (windows.go), so a grant for one add-on is no
// other's.
const profilePrefix = "sona.addon."

// idExpr is what an add-on ID looks like (addons.idExpr: the manifest's rule).
// A name in a folder that is not one is no add-on's, so no profile of one.
var idExpr = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)

// ProfileName is the AppContainer profile of the add-on id.
func ProfileName(id string) string { return profilePrefix + id }

// ProfileIDs are the IDs of the add-ons that may have an AppContainer profile
// on this machine, sorted and without repeats: bundled (Sona's own, always
// possible), and the name of every folder in dirs that is an ID. The caller
// passes the folders that name add-ons: the folder of the installed ones (a
// folder per add-on) and the state folder that holds a data folder per add-on
// that ever ran (which is never removed, so an add-on that was uninstalled
// still shows there, and so does one run by `sona addons dev`). A folder that
// can't be read names none.
func ProfileIDs(bundled []string, dirs ...string) []string {
	ids := slices.Clone(bundled)
	for _, dir := range dirs {
		des, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range des {
			if de.IsDir() && idExpr.MatchString(de.Name()) {
				ids = append(ids, de.Name())
			}
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}
