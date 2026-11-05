package kit

import "github.com/coredev-uk/sona-go/internal/ui/i18n"

// KeyringCopy is what the notices about the system's secret store say, in
// the words of the OS's own: the Secret Service (GNOME Keyring, KWallet) on
// Linux, the Keychain on macOS, Windows Credential Manager on Windows.
// Account and Settings share it so no notice tells a Mac user to unlock
// GNOME Keyring.
type KeyringCopy struct {
	// Waiting: the stored sign-in is still being read.
	Waiting string
	// Refused: the keyring didn't answer or refused, so Sona is signed out.
	Refused string
	// SaveRefused: it refused a sign-in, so nothing was saved.
	SaveRefused string
	// NotOpened: there is no keyring, so a sign-in would go to the file.
	NotOpened string
	// TokenInFile: signed in, with the token in the 0600 file.
	TokenInFile string
	// TokenInFileDiag is TokenInFile as a Diagnostics row's detail.
	TokenInFileDiag string
	// ScrobblerInFile: the scrobbler sign-ins are in files.
	ScrobblerInFile string
}

// KeyringWords is the copy for goos (runtime.GOOS, or Capabilities.OS); an
// OS it doesn't know, and "", get Linux's.
func KeyringWords(goos string) KeyringCopy {
	switch goos {
	case "darwin":
		return KeyringCopy{
			Waiting:         i18n.T("Sona is waiting for your Keychain to hand over your sign-in. If macOS asks you to unlock it, do that now."),
			Refused:         i18n.T("Your Keychain didn't answer or refused, so you're signed out. Unlock your login keychain in Keychain Access, then try again."),
			SaveRefused:     i18n.T("Your Keychain didn't answer or refused, so nothing was saved. Unlock your login keychain in Keychain Access, then sign in again."),
			NotOpened:       i18n.T("Sona couldn't open your Keychain, so it would keep your Apple Music token in a file only your user can read. Unlock your login keychain in Keychain Access, then try again."),
			TokenInFile:     i18n.T("Sona couldn't use your Keychain, so it keeps your Apple Music token in a file only your user can read. Unlock your login keychain in Keychain Access and Sona moves it into the Keychain."),
			TokenInFileDiag: i18n.T("Your Apple Music token is in a file only your user can read, not in the Keychain. Unlock your login keychain in Keychain Access and Sona moves it there."),
			ScrobblerInFile: i18n.T("Sona couldn't use your Keychain, so it keeps them in files only your user can read. Unlock your login keychain in Keychain Access and Sona moves them into the Keychain."),
		}
	case "windows":
		return KeyringCopy{
			Waiting:         i18n.T("Sona is waiting for Windows Credential Manager to hand over your sign-in."),
			Refused:         i18n.T("Windows Credential Manager didn't answer or refused, so you're signed out. Try again."),
			SaveRefused:     i18n.T("Windows Credential Manager didn't answer or refused, so nothing was saved. Sign in again."),
			NotOpened:       i18n.T("Sona couldn't open Windows Credential Manager, so it would keep your Apple Music token in a file only your user can read. Try again."),
			TokenInFile:     i18n.T("Sona couldn't use Windows Credential Manager, so it keeps your Apple Music token in a file only your user can read. Sona moves it into Credential Manager once that works."),
			TokenInFileDiag: i18n.T("Your Apple Music token is in a file only your user can read, not in Windows Credential Manager. Sona moves it there once that works."),
			ScrobblerInFile: i18n.T("Sona couldn't use Windows Credential Manager, so it keeps them in files only your user can read. Sona moves them into Credential Manager once that works."),
		}
	}
	return KeyringCopy{
		Waiting:         i18n.T("Sona is waiting for your keyring to hand over your sign-in. If GNOME Keyring or KWallet asks you to unlock it, do that now."),
		Refused:         i18n.T("Your keyring didn't answer or refused, so you're signed out. Unlock GNOME Keyring or KWallet, then try again."),
		SaveRefused:     i18n.T("Your keyring didn't answer or refused, so nothing was saved. Unlock GNOME Keyring or KWallet, then sign in again."),
		NotOpened:       i18n.T("Sona couldn't open a keyring, so it would keep your Apple Music token in a file only your user can read. Start or unlock GNOME Keyring or KWallet, then try again."),
		TokenInFile:     i18n.T("No Secret Service keyring is running, so Sona keeps your Apple Music token in a file only your user can read. Start GNOME Keyring or KWallet and Sona moves it into the keyring."),
		TokenInFileDiag: i18n.T("Your Apple Music token is in a file only your user can read, not in a keyring. Start GNOME Keyring or KWallet and Sona moves it there."),
		ScrobblerInFile: i18n.T("No Secret Service keyring is running, so Sona keeps them in files only your user can read. Start GNOME Keyring or KWallet and Sona moves them into the keyring."),
	}
}
