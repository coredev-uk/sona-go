## Installing

Every file is listed in `SHA256SUMS` and has signed build provenance:
`gh attestation verify <file> --repo coredev-uk/sona-go`.

**macOS (Apple silicon, macOS 14 or later).** Open `Sona-<version>-macos-arm64.dmg` and drag Sona to Applications. The app is not notarised, so macOS 15 and later block the first open: System Settings → Privacy & Security → "Open Anyway", or run `xattr -dr com.apple.quarantine /Applications/Sona.app`. Playback of previews works; sign in with Apple Music from Settings.

**Windows (x64).** Run `Sona-<version>-windows-x64-setup.exe` (a per-user install, no administrator rights) or extract the portable `Sona-<version>-windows-x64.zip` anywhere you can write. SmartScreen warns about an unknown publisher: "More info" → "Run anyway". Windows has no audio playback yet; browsing, your library and motion covers as stills work.

**Linux.** The `.deb`, `.rpm`, Arch package and AppImage; see the README.

Installs of the macOS app and the Windows installer or zip update themselves from Settings → Updates; packages update through your package manager.
