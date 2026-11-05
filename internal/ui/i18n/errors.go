package i18n

import "github.com/coredev-uk/sona-go/internal/services"

// ErrorText is the wording of a services error code in the language shown:
// v0.2.0's shared/lib/errors.ts codeMessages, with the same msgids. An
// unknown or empty code gets the generic message. Go's own error text is
// for logs and never reaches the UI.
func ErrorText(code services.ErrorCode) string {
	switch code {
	case services.CodeInvalidCredentials:
		return T("The account name or password isn't right.")
	case services.CodeIncorrectCode:
		return T("That verification code isn't right.")
	case services.CodeLocked:
		return T("This Apple Account is locked or temporarily unavailable.")
	case services.CodeActionRequired:
		return T("Your Apple Account needs attention at account.apple.com before it can sign in.")
	case services.CodeNoSubscription:
		return T("This Apple Account doesn't have an active Apple Music subscription.")
	case services.CodeUnsupportedCountry:
		return T("Apple Music isn't available in this account's country or region.")
	case services.CodeUnsupported2FA:
		return T("This account's two-factor method can't be completed inside Sona.")
	case services.CodeOutOfOrder:
		return T("The sign-in was replaced or cancelled. Start again.")
	case services.CodeTimeout:
		return T("That took too long and timed out. Try again.")
	case services.CodeCancelled:
		return T("It was cancelled.")
	case services.CodeNetwork:
		return T("Sona couldn't reach the network. Check your connection and try again.")
	case services.CodeMUTRejected:
		return T("Apple Music no longer accepts your sign-in. Sign in again from the account menu in the top bar.")
	case services.CodeDevTokenRejected:
		return T("Apple Music refused Sona's access. Try again later.")
	case services.CodeNotSignedIn:
		return T("Sign in to Apple Music from the account menu in the top bar.")
	case services.CodeUnauthorized:
		return T("Apple Music refused the request.")
	case services.CodeNotFound:
		return T("It couldn't be found. It may have been removed.")
	case services.CodeNotPlayable:
		return T("This song isn't available to stream in your storefront.")
	case services.CodeLiveStation:
		return T("This isn't available while listening to a live station.")
	case services.CodeStation:
		return T("This isn't available while listening to a station.")
	case services.CodeNotReleased:
		return T("This song isn't out yet. It plays from its release date.")
	case services.CodeSyncRunning:
		return T("A library sync is already running.")
	case services.CodePrivateForced:
		return T("Private listening stays on because Sona was started with SONA_PRIVATE=1.")
	case services.CodeInvalidArgument:
		return T("That value isn't valid.")
	case services.CodeNameRequired:
		return T("Enter a name.")
	case services.CodeFolderNotEmpty:
		return T("The folder isn't empty. Move or delete what's in it first.")
	case services.CodeNotAppleMusicLink:
		return T("That isn't an Apple Music link.")
	case services.CodeClipboard:
		return T("Sona couldn't use the clipboard.")
	case services.CodeTokenRequired:
		return T("Paste your user token first.")
	case services.CodeBrowserOpen:
		return T("Sona couldn't open your web browser. Open the link yourself.")
	case services.CodeSignOutIncomplete:
		return T("Sona signed out, but couldn't remove everything it saved. Unlock your keyring, then sign out again.")
	case services.CodeStoredUnreadable:
		return T("Your saved sign-in couldn't be read, so Sona removed it. Sign in again.")
	case services.CodeSettingsNotExport:
		return T("That file isn't a Sona settings export.")
	case services.CodeSettingsNewerExport:
		return T("That file was exported by a newer version of Sona. Update Sona, then import it.")
	case services.CodeSettingsBadValue:
		return T("That file has a setting Sona can't use, so nothing changed.")
	case services.CodeSettingsRead:
		return T("Sona couldn't read that file.")
	case services.CodeSettingsWrite:
		return T("Sona couldn't save the file there.")
	case services.CodeNoRendition:
		return T("Apple doesn't offer this video in a format Sona can play.")
	case services.CodeConflict:
		return T("It changed elsewhere since Sona read it. Reload and try again.")
	case services.CodePinsFull:
		return T("You have six pins. Unpin one to make room.")
	case services.CodeNotConfigured:
		return T("This build of Sona has no account for that service.")
	case services.CodeUnsupported:
		return T("That isn't available on this platform.")
	case services.CodeNoCDM:
		return T("Sona needs Google's Widevine to play songs, and it isn't ready yet. Settings → Playback shows its progress.")
	case services.CodeVideoHDOnly:
		return T("Apple offers this video only in HD, which needs a hardware-protected player. Sona plays videos in standard definition.")
	case services.CodeVideoLicence:
		return T("Apple refused the licence for this video, or it can't be used on this computer.")
	case services.CodeUpdateSignature:
		return T("The update isn't signed with Sona's key, so it wasn't installed.")
	case services.CodeUpdateFailed:
		return T("The update couldn't be installed. Sona is unchanged.")
	case services.CodeKeyring:
		return T("Your keyring didn't answer or refused. Unlock it, then try again.")
	case services.CodeImageUnusable:
		return T("Sona can't use that image. It must be a JPEG or PNG within the size limits.")
	}
	return T("Something went wrong.")
}

// ErrorMessage is the wording of an error for the user: Apple's own,
// already localised wording of a sign-in failure Sona has no code for,
// else the text of the error's code (services.CodeOf). nil is "".
func ErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if m := services.AppleMessage(err); m != "" {
		return m
	}
	return ErrorText(services.CodeOf(err))
}
