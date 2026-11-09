; Sona's per-user installer (build/windows/package.sh runs makensis on it).
; Installs into %LOCALAPPDATA%\Programs\Sona without administrator rights,
; with a Start-menu shortcut, the music: and itms: URL handlers and an
; uninstaller listed under Settings → Apps. Settings, the library database
; and sign-in live in %APPDATA%\Sona and %LOCALAPPDATA%\Sona and stay on
; uninstall; so does the Widevine CDM there. The uninstaller runs
; `sona.exe --remove-sandbox` first, which removes the CDM host's
; AppContainer profile and the access list entries granted to it. The
; folder installed is package.sh's: sona.exe, the CDM host's shim DLL, the
; GStreamer DLLs, lib\gstreamer-1.0 (the plugins), libexec\gstreamer-1.0
; (the plugin scanner) and licenses\, never the Widevine CDM, which Sona
; downloads into the user's data folder.
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

!macro UrlScheme scheme
  WriteRegStr HKCU "Software\Classes\${scheme}" "" "URL:${scheme} (Sona)"
  WriteRegStr HKCU "Software\Classes\${scheme}" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\${scheme}\DefaultIcon" "" "$INSTDIR\sona.exe,0"
  WriteRegStr HKCU "Software\Classes\${scheme}\shell\open\command" "" '"$INSTDIR\sona.exe" "%1"'
!macroend

Section "Sona"
  SetOutPath "$INSTDIR"
  File /r "${SRCDIR}\*.*"
  File "sona.ico"
  WriteUninstaller "$INSTDIR\uninstall.exe"

  CreateShortCut "$SMPROGRAMS\Sona.lnk" "$INSTDIR\sona.exe" "" "$INSTDIR\sona.ico"

  WriteRegStr HKCU "Software\Sona" "InstallDir" "$INSTDIR"
  !insertmacro UrlScheme "music"
  !insertmacro UrlScheme "itms"

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
  ; The CDM host's sandbox: Sona's AppContainer profile (with its package
  ; folder under %LOCALAPPDATA%\Packages) and the access list entries it was
  ; granted on the program's, GStreamer's and the Widevine folders. Sona
  ; removes them itself, before its files go. A failure is shown in the
  ; details and never stops the uninstall.
  IfFileExists "$INSTDIR\sona.exe" 0 sandboxdone
  ClearErrors
  ExecWait '"$INSTDIR\sona.exe" --remove-sandbox' $0
  IfErrors 0 +2
    StrCpy $0 "could not run sona.exe"
  DetailPrint "Sona's playback sandbox cleanup finished: $0"
  sandboxdone:
  Delete "$SMPROGRAMS\Sona.lnk"
  DeleteRegKey HKCU "Software\Classes\music"
  DeleteRegKey HKCU "Software\Classes\itms"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
  DeleteRegKey HKCU "Software\Sona"
  ; The folder holds only what the installer put there, plus what the
  ; updater leaves beside it (previous\).
  RMDir /r "$INSTDIR"
SectionEnd
