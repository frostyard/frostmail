; Frostmail's hooks into Tauri's NSIS installer (docs/design/desktop.md,
; Windows). The installer is per user; besides the app it installs maild,
; mailctl and frostmail-notify beside it, and the hooks run them.
;
; maild runs from the install directory, which locks maild.exe, and the
; app starts maild whenever it finds no maild running. A running file can
; still be renamed, so the hooks move each sidecar aside before stopping
; maild: the copy then writes fresh files, and an app still open cannot
; start the old maild again before the installer closes it.

!macro FROSTMAIL_ASIDE
  !insertmacro FROSTMAIL_ASIDE_ONE "maild.exe"
  !insertmacro FROSTMAIL_ASIDE_ONE "mailctl.exe"
  !insertmacro FROSTMAIL_ASIDE_ONE "frostmail-notify.exe"
  ; One maild per user; a forced stop is one maild's journal recovers from.
  ReadEnvStr $1 USERNAME
  nsExec::Exec '"$SYSDIR\taskkill.exe" /F /IM maild.exe /FI "USERNAME eq $1"'
  Pop $0
!macroend

!macro FROSTMAIL_ASIDE_ONE name
  Delete "$INSTDIR\${name}.old"
  ClearErrors
  Rename "$INSTDIR\${name}" "$INSTDIR\${name}.old"
  ClearErrors
!macroend

!macro FROSTMAIL_ASIDE_GONE
  Delete /REBOOTOK "$INSTDIR\maild.exe.old"
  Delete /REBOOTOK "$INSTDIR\mailctl.exe.old"
  Delete /REBOOTOK "$INSTDIR\frostmail-notify.exe.old"
!macroend

!macro NSIS_HOOK_PREINSTALL
  !insertmacro FROSTMAIL_ASIDE
!macroend

!macro NSIS_HOOK_POSTINSTALL
  !insertmacro FROSTMAIL_ASIDE_GONE
  ; Toasts: Windows shows them for an AppUserModelID it knows. The Start
  ; Menu shortcut declares the bundle identifier; so does this key, for an
  ; install made without shortcuts (/NS).
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${BUNDLEID}" "DisplayName" "${PRODUCTNAME}"
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${BUNDLEID}" "IconUri" "$INSTDIR\frostmail.png"
  ; A click on a toast, or its Snooze or Dismiss, is a frostmail-toast: URI
  ; that Windows hands to the notifier.
  WriteRegStr HKCU "Software\Classes\frostmail-toast" "" "URL:${PRODUCTNAME} notification"
  WriteRegStr HKCU "Software\Classes\frostmail-toast" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\frostmail-toast\DefaultIcon" "" "$\"$INSTDIR\${MAINBINARYNAME}.exe$\",0"
  WriteRegStr HKCU "Software\Classes\frostmail-toast\shell\open\command" "" "$\"$INSTDIR\frostmail-notify.exe$\" --activate $\"%1$\""
  ; maild starts at sign-in (the Run key's Frostmail value, which the
  ; uninstaller removes) and now, as the user even from an elevated
  ; installer, so that mail and reminders notify with the app closed.
  nsExec::Exec '"$INSTDIR\maild.exe" -autostart on'
  Pop $0
  nsis_tauri_utils::RunAsUser "$INSTDIR\maild.exe" ""
!macroend

!macro NSIS_HOOK_PREUNINSTALL
  !insertmacro FROSTMAIL_ASIDE
  ; An update runs the old uninstaller first and keeps what the user set up.
  ${If} $UpdateMode <> 1
    DeleteRegKey HKCU "Software\Classes\frostmail-toast"
    DeleteRegKey HKCU "Software\Classes\AppUserModelId\${BUNDLEID}"
  ${EndIf}
!macroend

!macro NSIS_HOOK_POSTUNINSTALL
  !insertmacro FROSTMAIL_ASIDE_GONE
  ; "Delete the application data" also deletes maild's: the mail, the
  ; accounts and their sealed secrets, the parts cache and the logs.
  ${If} $DeleteAppDataCheckboxState = 1
  ${AndIf} $UpdateMode <> 1
    RMDir /r "$LOCALAPPDATA\${PRODUCTNAME}\Data"
    RMDir /r "$LOCALAPPDATA\${PRODUCTNAME}\Cache"
    RMDir "$LOCALAPPDATA\${PRODUCTNAME}"
  ${EndIf}
  ; The template removes the install directory only if empty, which it was
  ; not while the moved-aside files or maild's data were in it.
  RMDir "$INSTDIR"
!macroend
