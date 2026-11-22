; Sona's per-user installer (build/windows/package.sh runs makensis on it).
; Installs into %LOCALAPPDATA%\Programs\Sona without administrator rights,
; with a Start-menu shortcut, the music:, musics:, itms: and itmss: URL handlers and an
; uninstaller listed under Settings → Apps. Settings, the library database
; and sign-in live in %APPDATA%\Sona and %LOCALAPPDATA%\Sona and stay on
; uninstall; so does the Widevine CDM there. The uninstaller runs
; `sona.exe --remove-sandbox` first, which removes the CDM host's
; and the add-ons' AppContainer profiles and the access list entries granted to them. The
; folder installed is package.sh's: sona.exe, the CDM host's shim DLL, the
; GStreamer DLLs, lib\gstreamer-1.0 (the plugins), libexec\gstreamer-1.0
; (the plugin scanner) and licenses\, never the Widevine CDM, which Sona
; downloads into the user's data folder. Neither the installer nor the
; uninstaller runs while Sona does, and an install over a folder where an
; update kept the previous version drops that version (docs/architecture.md,
; "Rollback guard").
;
; Defines (makensis -D): VERSION, VERSION_NUMERIC, SRCDIR (the portable
; folder), OUTFILE.
Unicode true
ManifestDPIAware true
RequestExecutionLevel user
SetCompressor /SOLID lzma

!include "MUI2.nsh"

Name "Sona"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\Programs\Sona"
InstallDirRegKey HKCU "Software\Sona" "InstallDir"
BrandingText "Sona ${VERSION}"
VIProductVersion "${VERSION_NUMERIC}"
VIAddVersionKey "ProductName" "Sona"
VIAddVersionKey "FileDescription" "Sona installer"
VIAddVersionKey "FileVersion" "${VERSION}"
VIAddVersionKey "LegalCopyright" "Sona contributors"

!define MUI_ICON "sona.ico"
!define MUI_UNICON "sona.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\sona.exe"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Sona"
!define RUN_KEY "Software\Microsoft\Windows\CurrentVersion\Run"
!define APPROVED_KEY "Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run"

; The Apple Music link schemes (app.LinkSchemes), each a class of the user's
; whose open command runs this installation, the way MyGo's
; RegisterURLScheme writes it (Settings → General → Open Apple Music links
; in Sona; cmd/sona/linkhandler_windows.go).
!macro UrlScheme scheme
  WriteRegStr HKCU "Software\Classes\${scheme}" "" "URL:${scheme} (Sona)"
  WriteRegStr HKCU "Software\Classes\${scheme}" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\${scheme}\DefaultIcon" "" "$INSTDIR\sona.exe,0"
  WriteRegStr HKCU "Software\Classes\${scheme}\shell\open\command" "" '"$INSTDIR\sona.exe" "%1"'
!macroend

; Only a class that runs this installation goes: a portable copy may own the
; scheme now (StrCmp ignores case).
!macro UnUrlScheme scheme
  ReadRegStr $1 HKCU "Software\Classes\${scheme}\shell\open\command" ""
  StrCmp $1 '"$INSTDIR\sona.exe" "%1"' 0 +2
    DeleteRegKey HKCU "Software\Classes\${scheme}"
!macroend

; Sona's state folder, as the app resolves it (config.ResolvePaths: the
; LOCALAPPDATA variable, which $LOCALAPPDATA, the shell folder, need not be).
!macro StateDir var
  ReadEnvStr ${var} LOCALAPPDATA
  StrCpy ${var} "${var}\Sona\State"
!macroend

; Sona runs while its single-instance lock is held: the first byte of
; sona.lock in the state folder, locked with LockFileEx (internal/app,
; socket_windows.go). Taking the same lock without waiting tells: $R9 is 1
; when another process holds it. Any Sona of this user counts, the installed
; one, a portable copy or one in the tray, since they share the state folder.
; Labels are local to a function or section, and each inserts this once.
!macro SonaRunning
  StrCpy $R9 0
  !insertmacro StateDir $R8
  IfFileExists "$R8\sona.lock" 0 sonarunning_done
  ; GENERIC_READ|GENERIC_WRITE, FILE_SHARE_READ|WRITE|DELETE, OPEN_EXISTING.
  System::Call 'kernel32::CreateFileW(w "$R8\sona.lock", i 0xC0000000, i 7, p 0, i 3, i 0x80, p 0) p .R7'
  IntPtrCmp $R7 -1 sonarunning_done
  System::Alloc 32 ; a zeroed OVERLAPPED: the lock's offset is 0
  Pop $R6
  ; LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY, one byte, as tryLock.
  System::Call 'kernel32::LockFileEx(p R7, i 3, i 0, i 1, i 0, p R6) i .R5'
  IntCmp $R5 0 sonarunning_held
  System::Call 'kernel32::UnlockFileEx(p R7, i 0, i 1, i 0, p R6)'
  Goto sonarunning_close
  sonarunning_held:
  StrCpy $R9 1
  sonarunning_close:
  System::Free $R6
  System::Call 'kernel32::CloseHandle(p R7)'
  sonarunning_done:
!macroend

; Replacing or removing Sona's files under a running Sona would leave a mix
; of versions (a file in use can't be overwritten): ask the user to quit it
; first. A silent run doesn't wait: it stops (exit code 2) and changes
; nothing.
!macro WaitForSonaToQuit
  sonaquit_retry:
  !insertmacro SonaRunning
  IntCmp $R9 0 sonaquit_done
  MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "Sona is running. Quit it (Quit in its tray icon's menu, or in the app), then choose Retry." /SD IDCANCEL IDRETRY sonaquit_retry
  Abort
  sonaquit_done:
!macroend

Function .onInit
  !insertmacro WaitForSonaToQuit
FunctionEnd

Function un.onInit
  !insertmacro WaitForSonaToQuit
FunctionEnd

Section "Sona"
  ; Sona may have been started since the installer was.
  !insertmacro WaitForSonaToQuit
  ; The version an update kept for Settings → Diagnostics → Previous version
  ; (internal/update's guard) is the one the update replaced, and the
  ; version installed here replaces both: restoring it would put older files
  ; over these. Drop it with its number, and any update still on trial.
  IfFileExists "$INSTDIR\previous\*.*" 0 previousdone
  RMDir /r "$INSTDIR\previous"
  !insertmacro StateDir $R8
  Delete "$R8\update-previous"
  Delete "$R8\update-pending"
  previousdone:
  RMDir /r "$INSTDIR\previous.staging"
  RMDir /r "$INSTDIR\previous.old"
  SetOutPath "$INSTDIR"
  File /r "${SRCDIR}\*.*"
  File "sona.ico"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateShortCut "$SMPROGRAMS\Sona.lnk" "$INSTDIR\sona.exe" "" "$INSTDIR\sona.ico"

  WriteRegStr HKCU "Software\Sona" "InstallDir" "$INSTDIR"
  !insertmacro UrlScheme "music"
  !insertmacro UrlScheme "musics"
  !insertmacro UrlScheme "itms"
  !insertmacro UrlScheme "itmss"

  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "Sona"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "Sona"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\sona.ico"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr HKCU "${UNINSTALL_KEY}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
SectionEnd

Section "Uninstall"
  ; The CDM host's and the add-ons' sandboxes: Sona's AppContainer profiles
  ; (with their package folders under %LOCALAPPDATA%\Packages) and the access
  ; list entries they were granted on the program's, GStreamer's, the Widevine
  ; and the add-ons' folders. Sona
  ; removes them itself, before its files go. A failure is shown in the
  ; details and never stops the uninstall.
  IfFileExists "$INSTDIR\sona.exe" 0 sandboxdone
  ClearErrors
  ExecWait '"$INSTDIR\sona.exe" --remove-sandbox' $0
  IfErrors 0 +2
    StrCpy $0 "could not run sona.exe"
  DetailPrint "Sona's sandbox cleanup finished: $0"
  sandboxdone:
  Delete "$SMPROGRAMS\Sona.lnk"
  !insertmacro UnUrlScheme "music"
  !insertmacro UnUrlScheme "musics"
  !insertmacro UnUrlScheme "itms"
  !insertmacro UnUrlScheme "itmss"
  ; Start at login (Settings → General): MyGo writes a value named after the
  ; app in the user's Run key, and Task Manager's switch for it in
  ; StartupApproved (cmd/sona/login_desktop.go). Only this installation's
  ; entry goes: a portable copy may own the value (StrCmp ignores case).
  ReadRegStr $1 HKCU "${RUN_KEY}" "Sona"
  StrCmp $1 '"$INSTDIR\sona.exe" --mygo-opened-at-login' 0 logindone
  DeleteRegValue HKCU "${RUN_KEY}" "Sona"
  DeleteRegValue HKCU "${APPROVED_KEY}" "Sona"
  logindone:
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  DeleteRegKey HKCU "Software\Sona"
  ; The folder holds only what the installer put there, plus what the
  ; updater leaves beside it (previous\).
  RMDir /r "$INSTDIR"
SectionEnd
