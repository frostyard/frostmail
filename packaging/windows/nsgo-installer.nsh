; Frostmail's additions to the NSIS installer that nsgo build writes for the
; NSGo client (docs/design/desktop.md, Windows; plan 0017, Phase 3). NSGo's
; script has no place for an app's own steps, so tools/nsgoinstaller writes
; this file into it, before .onInit, and its 3 macros at their places:
;   FM_INIT       in .onInit;
;   FM_INSTALL    at the end of the install section, once the files are in
;                 $INSTDIR;
;   FM_UNINSTALL  in the uninstall section, before the files go.
; The steps are those of installer.nsh, which MyGo's installer takes.
;
; The installer is per user. maild, mailctl and frostmail-notify ship beside
; the app as its resources; NSGo's CloseApp stops whatever runs from the
; install folder, maild among them, before it replaces the files.
;
; A check of the installer redefines the names below, so that it leaves a
; real install on the same machine alone (nsgo-installer-check.nsh).

!include "LogicLib.nsh"

!ifndef FM_NAME
  !define FM_NAME "FrostMail"
!endif
!ifndef FM_EXE
  !define FM_EXE "FrostMail.exe"
!endif
; The AppUserModelID the notifier posts its toasts under.
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

; Sets $R3 to 1 when the folders $R1 and $R2 are the same or one holds the
; other, else to "". The app's folder must not be, hold or sit in the folder
; with the mail, which the uninstaller deletes on a yes, nor the Tauri
; install's, whose files FM_TAKE_OVER deletes after the app's are copied.
Function FmOverlap
  StrCpy $R3 ""
  ${If} $R1 == ""
  ${OrIf} $R2 == ""
    Return
  ${EndIf}
  ; No trailing backslash, as a drive's root has.
  StrCpy $R4 $R1 "" -1
  ${If} $R4 == "\"
    StrCpy $R1 $R1 -1
  ${EndIf}
  StrCpy $R4 $R2 "" -1
  ${If} $R4 == "\"
    StrCpy $R2 $R2 -1
  ${EndIf}
  ; $R1 is the shorter.
  StrLen $R4 $R1
  StrLen $R5 $R2
  ${If} $R4 > $R5
    StrCpy $R6 $R1
    StrCpy $R1 $R2
    StrCpy $R2 $R6
    StrLen $R4 $R1
  ${EndIf}
  StrCpy $R6 $R2 $R4
  ${If} $R6 == $R1
    StrCpy $R6 $R2 1 $R4
    ${If} $R6 == ""
    ${OrIf} $R6 == "\"
      StrCpy $R3 "1"
    ${EndIf}
  ${EndIf}
FunctionEnd

; Sets $R3 to 1 when $INSTDIR overlaps the mail's folder or the Tauri install.
Function FmFolderRefused
  StrCpy $R1 $INSTDIR
  StrCpy $R2 "${FM_DATA_ROOT}"
  Call FmOverlap
  ${If} $R3 == ""
    StrCpy $R1 $INSTDIR
    StrCpy $R2 $FmOld
    Call FmOverlap
  ${EndIf}
FunctionEnd

; The directory page calls this as the user types: a refused folder leaves
; Next disabled.
Function .onVerifyInstDir
  Call FmFolderRefused
  ${If} $R3 != ""
    Abort
  ${EndIf}
FunctionEnd

; The Tauri app and its maild hold the mail's lock and their own files, so
; they stop first. In .onInit, where a user who says no leaves with nothing
; changed. /D= skips the directory page, so a refused folder ends it here.
!macro FM_INIT
  ReadRegStr $FmOld HKCU "${FM_TAURI_KEY}" "InstallLocation"
  ${If} $FmOld != ""
    ; Tauri writes it in quotes.
    StrCpy $R0 $FmOld 1
    ${If} $R0 == '"'
      StrCpy $FmOld $FmOld -1 1
    ${EndIf}
  ${EndIf}
  Call FmFolderRefused
  ${If} $R3 != ""
    MessageBox MB_OK|MB_ICONSTOP "${FM_NAME} cannot be installed in $INSTDIR: that folder holds your mail." /SD IDOK
    SetErrorLevel 2
    Quit
  ${EndIf}
  ${If} $FmOld != ""
    ; NSGo's CloseApp stops what runs from $INSTDIR.
    Push $INSTDIR
    StrCpy $INSTDIR $FmOld
    Call CloseApp
    Pop $INSTDIR
  ${EndIf}
!macroend

; Removes the Tauri app's own files and entries. Its folder also holds the
; mail, in Data and Cache, which stay where maild looks for them.
!macro FM_TAKE_OVER
  ${If} $FmOld != ""
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

!macro FM_INSTALL
  ; Toasts: Windows shows them for an AppUserModelID it knows.
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${FM_APP_ID}" "DisplayName" "${FM_NAME}"
  WriteRegStr HKCU "Software\Classes\AppUserModelId\${FM_APP_ID}" "IconUri" "$INSTDIR\frostmail.png"
  ; A click on a toast, or its Snooze or Dismiss, is a frostmail-toast: URI
  ; that Windows hands to the notifier.
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}" "" "URL:${FM_NAME} notification"
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}" "URL Protocol" ""
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}\DefaultIcon" "" "$\"$INSTDIR\${FM_EXE}$\",0"
  WriteRegStr HKCU "Software\Classes\${FM_TOAST_SCHEME}\shell\open\command" "" "$\"$INSTDIR\frostmail-notify.exe$\" --activate $\"%1$\""
  !insertmacro FM_TAKE_OVER
  ; MyGo's installer has this installer's uninstall entry, so this one
  ; installs into its folder, over its files of the same names. It held the
  ; NSGo client as frostmail-nsgo.exe, which is no file of this install, so
  ; this uninstaller would leave it.
  Delete "$INSTDIR\frostmail-nsgo.exe"
  ; maild starts at sign-in (the Run key's Frostmail value) and now, so
  ; that mail and reminders notify with the app closed.
  !ifndef FM_CHECK
    nsExec::Exec '"$INSTDIR\maild.exe" -autostart on'
    Pop $0
    Exec '"$INSTDIR\maild.exe"'
  !else
    ; A check's maild starts only on a pipe and a data folder of the
    ; check's own, which it takes from the installer's environment: with
    ; neither it would be the user's maild, on the user's mail.
    ReadEnvStr $0 FROSTMAIL_SOCKET
    ReadEnvStr $1 FROSTMAIL_DATA_DIR
    ${If} $0 != ""
    ${AndIf} $1 != ""
      Exec '"$INSTDIR\maild.exe"'
    ${EndIf}
  !endif
!macroend

!macro FM_UNINSTALL
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
