package module

import "github.com/coredev-uk/sona-go/internal/ui/i18n"

// ShortcutGroups are the shortcut groups in the order the palette, the help
// sheet and Settings list them (v0.2.0's shortcutGroups).
var ShortcutGroups = []ShortcutGroup{"playback", "view", "pages", "navigation", "general"}

// AddonsGroup is the palette's group of the commands add-ons contribute
// (Env.AddonCommands); it is not one of the shortcut groups, which have keys.
const AddonsGroup ShortcutGroup = "addons"

// GroupTitle is a group's heading in the palette, the help sheet and
// Settings (shared/shortcuts/groups.ts).
func GroupTitle(g ShortcutGroup) string {
	switch g {
	case "playback":
		return i18n.T("Playback")
	case "view":
		return i18n.T("View")
	case "pages":
		return i18n.T("Pages")
	case "navigation":
		return i18n.T("Navigation")
	case "general":
		return i18n.T("General")
	case AddonsGroup:
		return i18n.T("Add-ons")
	}
	return string(g)
}
