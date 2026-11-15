#!/bin/sh
# Pre-remove: unload the AppArmor profile (build/linux/apparmor/sona) while
# its file still exists; the package manager removes the file itself.
# Upgrades keep it: dpkg passes "upgrade", rpm a count above 0.
# Every step is optional; a missing tool is not an error.

case "$1" in
remove | purge | 0)
  if [ -f /etc/apparmor.d/sona ] && command -v apparmor_parser >/dev/null 2>&1 &&
    [ -d /sys/kernel/security/apparmor ]; then
    apparmor_parser -R /etc/apparmor.d/sona || :
  fi
  ;;
esac

exit 0
