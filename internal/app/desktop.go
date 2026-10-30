package app

// DesktopMimeTypes is the MimeType value dev.coredev.Sona.desktop needs so that
// music://, musics://, itms:// and itmss:// links open Sona. The desktop entry
// must pass URLs through, e.g.
//
//	[Desktop Entry]
//	Type=Application
//	Name=Sona
//	Exec=sona %U
//	Icon=sona
//	StartupWMClass=dev.coredev.Sona
//	Categories=Audio;AudioVideo;Music;
//	MimeType=x-scheme-handler/music;x-scheme-handler/musics;x-scheme-handler/itms;x-scheme-handler/itmss;
//
// https://music.apple.com links reach Sona only when the user opens them
// with Sona explicitly; claiming x-scheme-handler/https would hijack the
// browser. After installing, `update-desktop-database` refreshes the cache;
// `xdg-mime default dev.coredev.Sona.desktop x-scheme-handler/music` makes
// Sona the default handler.
const DesktopMimeTypes = "x-scheme-handler/music;x-scheme-handler/musics;x-scheme-handler/itms;x-scheme-handler/itmss;"
