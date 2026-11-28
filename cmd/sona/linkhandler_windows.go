package main

import (
	"strings"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows/registry"

	"github.com/coredev-uk/sona-go/internal/app"
)

// windowsLinks is the default app for Apple Music links on Windows
// (app.WindowsLinkHandler has the rules):
//
//   - MyGo's App.RegisterURLScheme and IsURLSchemeRegistered write and read
//     HKCU\Software\Classes\<scheme>, whose open command runs this
//     executable with the link (`"…\sona.exe" "%1"`, as the installer writes
//     it). A link then starts Sona with the URL as its argument, or a second
//     launch hands it to the running Sona over the single-instance socket.
//   - The user's choice in Windows' settings is read from UserChoice, which
//     no app can write.
//   - Shell.DefaultApp (AssocQueryString) names the app Windows resolves; an
//     AppUserModelID ("<package family>!<app>") is a packaged app.
func windowsLinks() app.WindowsLinkHandler {
	return app.WindowsLinkHandler{
		Registered: mygo.App.IsURLSchemeRegistered,
		Register:   mygo.App.RegisterURLScheme,
		Choice:     userChoice,
		Handler: func(scheme string) (string, bool) {
			a, err := mygo.Shell.DefaultApp(scheme + "://music.apple.com/")
			if err != nil {
				return "", false
			}
			return a.Name, strings.Contains(a.ID, "!")
		},
	}
}

// userChoice is the ProgId of the app the user chose for scheme in Windows'
// settings; "" when they chose none.
func userChoice(scheme string) string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\`+scheme+`\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return ""
	}
	return v
}
