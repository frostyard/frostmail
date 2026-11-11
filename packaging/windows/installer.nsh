; Frostmail's additions to MyGo's NSIS installer (docs/design/desktop.md,
; Windows). The installer is per user. maild, mailctl and frostmail-notify
; ship beside the app as its resources; MyGo's CloseApp stops whatever runs
; from the install folder, maild among them, before it replaces the files.
;
; A check of the installer redefines the names below, so that it leaves a
; real install on the same machine alone (plan 0015, Phase 9).

!include "LogicLib.nsh"

!ifndef FM_NAME
  !define FM_NAME "Frostmail"
!endif
!ifndef FM_EXE
  !define FM_EXE "Frostmail.exe"
!endif
!ifndef FM_APP_ID
  !define FM_APP_ID "org.frostyard.Frostmail"
!endif
!ifndef FM_TOAST_SCHEME
  !define FM_TOAST_SCHEME "frostmail-toast"
!endif
; The Tauri installer's entry in Settings > Apps, whose install folder this
; installer takes over.
!ifndef FM_TAURI_KEY
  !define FM_TAURI_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Frostmail"
!endif
!ifndef FM_TAURI_VENDOR_KEY
  !define FM_TAURI_VENDOR_KEY "Software\frostyard"
!endif
; Where maild keeps the mail, whichever installer put it there.
!ifndef FM_DATA_ROOT
  !define FM_DATA_ROOT "$LOCALAPPDATA\Frostmail"
!endif
; What a silent uninstall answers to deleting the mail.
!ifndef FM_DELETE_SILENT
  !define FM_DELETE_SILENT "IDNO"
!endif

Var FmOld

; The Tauri app and its maild hold the mail's lock and their own files, so
; they stop first. In .onInit, where a user who says no leaves with nothing
; changed.
!macro customInit
  ReadRegStr $FmOld HKCU "${FM_TAURI_KEY}" "InstallLocation"
  ${If} $FmOld != ""
    ; Tauri writes it in quotes.
    StrCpy $R0 $FmOld 1
    ${If} $R0 == '"'
      StrCpy $FmOld $FmOld -1 1
    ${EndIf}
    Push $FmOld
    Call CloseApp
  ${EndIf}
!macroend

; Removes the Tauri app's own files and entries. Its folder also holds the
; mail, in Data and Cache, which stay where maild looks for them.
!macro FROSTMAIL_TAKE_OVER
  ${If} $FmOld != ""
  ${AndIf} $FmOld != $INSTDIR
    Delete "$FmOld\frostmail.exe"
    Delete "$FmOld\maild.exe"
    Delete "$FmOld\mailctl.exe"
    Delete "$FmOld\frostmail-notify.exe"
    Delete "$FmOld\frostmail.png"
    Delete "$FmOld\uninstall.exe"
    Delete "$FmOld\maild.exe.old"
    Delete "$FmOld\mailctl.exe.old"
    Delete "$FmOld\frostmail-notify.exe.old"
    RMDir "$FmOld"
    DeleteRegKey HKCU "${FM_TAURI_KEY}"
    DeleteRegKey HKCU "${FM_TAURI_VENDOR_KEY}\Frostmail"
    DeleteRegKey /ifempty HKCU "${FM_TAURI_VENDOR_KEY}"
    ; A desktop shortcut of the old installer names a file just deleted.
    IfFileExists "$DESKTOP\${FM_NAME}.lnk" 0 +2
    Call CreateDesktopShortcut
  ${EndIf}
!macroend

!macro customInstall
  ; Toasts: Windows shows them for an AppUserModelID it knows.
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${FM_APP_ID}" "DisplayName" "${FM_NAME}"
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${FM_APP_ID}" "IconUri" "$INSTDIR\frostmail.png"
  ; A click on a toast, or its Snooze or Dismiss, is a frostmail-toast: URI
  ; that Windows hands to the notifier.
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}" "" "URL:${FM_NAME} notification"
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}\DefaultIcon" "" "$\"$INSTDIR\${FM_EXE}$\",0"
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}\shell\open\command" "" "$\"$INSTDIR\frostmail-notify.exe$\" --activate $\"%1$\""
  !insertmacro FROSTMAIL_TAKE_OVER
  ; maild starts at sign-in (the Run key's Frostmail value) and now, so
  ; that mail and reminders notify with the app closed.
  !ifndef FM_CHECK
    nsExec::Exec '"$INSTDIR\maild.exe" -autostart on'
    Pop $0
  !endif
  Exec '"$INSTDIR\maild.exe"'
!macroend

!macro customUnInstall
  DeleteRegKey HKCU "Software\Classes\${FM_TOAST_SCHEME}"
  DeleteRegKey HKCU "Software\Classes\AppUserModelId\${FM_APP_ID}"
  !ifndef FM_CHECK
    DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "Frostmail"
  !endif
  ; The mail, the accounts and their sealed secrets, the parts cache and
  ; the logs. A silent uninstall keeps them.
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "Also delete your mail and accounts from this computer?" /SD ${FM_DELETE_SILENT} IDNO fm_keep_data
  RMDir /r "${FM_DATA_ROOT}\Data"
  RMDir /r "${FM_DATA_ROOT}\Cache"
  RMDir "${FM_DATA_ROOT}"
  fm_keep_data:
!macroend
