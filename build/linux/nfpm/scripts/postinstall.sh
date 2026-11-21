#!/bin/sh
# Post-install: refresh the caches that make the desktop
# entry, its music:// / itms:// scheme handlers and the icons visible, and
# load the AppArmor profile (build/linux/apparmor/sona) that lets Sona's
# playback sandbox create its user namespaces under Ubuntu's restriction.
# Every step is optional; a missing tool is not an error.

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database -q /usr/share/applications || :
fi

if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q -t -f /usr/share/icons/hicolor || :
fi

# Load the profile now, not at the next boot. A system without AppArmor, or
# one that does not enforce it, has no use for it.
if [ -f /etc/apparmor.d/sona ] && command -v apparmor_parser >/dev/null 2>&1 &&
  [ -d /sys/kernel/security/apparmor ]; then
  apparmor_parser -r -T -W /etc/apparmor.d/sona || :
fi

exit 0
