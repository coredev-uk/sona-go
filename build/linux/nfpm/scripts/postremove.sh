#!/bin/sh
# Post-remove: refresh the caches that listed the desktop entry, its
# music:// / itms:// scheme handlers and the icons. A separate script from
# postinstall.sh, which also loads the AppArmor profile: dpkg keeps the
# profile's file after a plain "remove", and loading it again here would undo
# preremove.sh. Every step is optional; a missing tool is not an error.

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || :
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || :
fi

exit 0
