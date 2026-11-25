; Written by nsgo build. Compile with NSIS 3.06 or later: makensis -INPUTCHARSET UTF8 FrostMail.exe.nsi
Unicode true
; RemoveFileTypes keeps an extension's key while it holds another app's values, and DeleteRegKey /ifempty only checks values from NSIS 3.06: an older compiler's uninstaller would delete them.
!if ${NSIS_PACKEDVERSION} < 0x03006000
!error "Compile with NSIS 3.06 or later"
!endif
ManifestDPIAware true
SetCompressor /SOLID lzma
Name "FrostMail" "FrostMail"
OutFile "frostmail-0.9.0-windows-x64-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\FrostMail"
; Install again where the app is, the folder the user chose before.
InstallDirRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "InstallLocation"
RequestExecutionLevel user
BrandingText " "
!define MUI_ICON "FrostMail-setup.ico"
!define MUI_UNICON "FrostMail-uninstall.ico"
!define MUI_FINISHPAGE_RUN "$INSTDIR\FrostMail.exe"
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!define MUI_FINISHPAGE_SHOWREADME_TEXT "$(NSGO_SHORTCUT)"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION CreateDesktopShortcut
!include "MUI2.nsh"
!include "x64.nsh"
!include "FileFunc.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"
LangString NSGO_SHORTCUT ${LANG_English} "Create a desktop shortcut"
LangString NSGO_RUNNING ${LANG_English} "FrostMail is running. Close it and continue?"
VIProductVersion "0.9.0.0"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductName" "FrostMail"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" "0.9.0"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileVersion" "0.9.0"
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileDescription" "FrostMail Setup"

Function .onInit
FunctionEnd

; Quits the app running from $INSTDIR: asks first unless silent.
!macro CloseApp un
Function ${un}CloseApp
  Push $0
  Push $1
  StrCpy $0 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $0 +2
  StrCpy $0 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $0 0 done
  System::Call 'Kernel32::SetEnvironmentVariable(t "NSGO_APP_DIR", t "$INSTDIR")'
  IfSilent close
  System::Call 'Kernel32::SetEnvironmentVariable(t "NSGO_APP_CHECK", t "1")'
  nsExec::Exec `"$0" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$$d = $$env:NSGO_APP_DIR.TrimEnd('\') + '\'; $$p = @(Get-CimInstance Win32_Process | Where-Object { $$_.ExecutablePath -and $$_.ExecutablePath.StartsWith($$d, 'OrdinalIgnoreCase') }); if ($$env:NSGO_APP_CHECK) { if ($$p.Count) { exit 2 }; exit 0 }; $$p | ForEach-Object { Stop-Process -Id $$_.ProcessId -Force -ErrorAction SilentlyContinue }; Start-Sleep -Milliseconds 500"`
  Pop $1
  System::Call 'Kernel32::SetEnvironmentVariable(t "NSGO_APP_CHECK", t "")'
  StrCmp $1 "2" 0 done
  MessageBox MB_OKCANCEL|MB_ICONEXCLAMATION "$(NSGO_RUNNING)" IDOK close
  Quit
  close:
  nsExec::Exec `"$0" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$$d = $$env:NSGO_APP_DIR.TrimEnd('\') + '\'; $$p = @(Get-CimInstance Win32_Process | Where-Object { $$_.ExecutablePath -and $$_.ExecutablePath.StartsWith($$d, 'OrdinalIgnoreCase') }); if ($$env:NSGO_APP_CHECK) { if ($$p.Count) { exit 2 }; exit 0 }; $$p | ForEach-Object { Stop-Process -Id $$_.ProcessId -Force -ErrorAction SilentlyContinue }; Start-Sleep -Milliseconds 500"`
  Pop $1
  done:
  Pop $1
  Pop $0
FunctionEnd
!macroend
!insertmacro CloseApp ""
!insertmacro CloseApp "un."

; Remove only this installation's registrations, including types and extensions dropped in an upgrade. A changed command belongs to someone else; preserve it and its Open With entries.
!macro RemoveFileTypes un
Function ${un}RemoveFileTypes
  Push $0
  Push $1
  Push $2
  Push $3
  Push $4
  next_type:
  EnumRegKey $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\FileTypes" 0
  StrCmp $0 "" done
  ReadRegStr $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\FileTypes\$0" ""
  ReadRegStr $2 HKCU "Software\Classes\$0\shell\open\command" ""
  StrCmp $1 "" forget_type
  StrCmp $1 $2 0 forget_type
  StrCpy $4 0
  next_extension:
  ClearErrors
  EnumRegValue $3 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\FileTypes\$0" $4
  IfErrors remove_type
  IntOp $4 $4 + 1
  StrCmp $3 "" next_extension
  DeleteRegValue HKCU "Software\Classes\.$3\OpenWithProgids" "$0"
  DeleteRegKey /ifempty HKCU "Software\Classes\.$3\OpenWithProgids"
  DeleteRegKey /ifempty HKCU "Software\Classes\.$3"
  Goto next_extension
  remove_type:
  DeleteRegKey HKCU "Software\Classes\$0"
  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'
  forget_type:
  ClearErrors
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\FileTypes\$0"
  IfErrors done
  Goto next_type
  done:
  DeleteRegKey /ifempty HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\FileTypes"
  Pop $4
  Pop $3
  Pop $2
  Pop $1
  Pop $0
FunctionEnd
!macroend
!insertmacro RemoveFileTypes ""
!insertmacro RemoveFileTypes "un."

; Remove the URL schemes this installation registered, including those an upgrade dropped. A scheme whose command changed belongs to someone else now; keep it.
!macro RemoveURLSchemes un
Function ${un}RemoveURLSchemes
  Push $0
  Push $1
  Push $2
  next_scheme:
  ClearErrors
  EnumRegValue $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\URLSchemes" 0
  IfErrors done
  StrCmp $0 "" done
  ReadRegStr $1 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\URLSchemes" $0
  ReadRegStr $2 HKCU "Software\Classes\$0\shell\open\command" ""
  StrCmp $1 $2 0 forget_scheme
  DeleteRegKey HKCU "Software\Classes\$0"
  forget_scheme:
  ClearErrors
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\URLSchemes" $0
  IfErrors done
  Goto next_scheme
  done:
  DeleteRegKey /ifempty HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail\URLSchemes"
  Pop $2
  Pop $1
  Pop $0
FunctionEnd
!macroend
!insertmacro RemoveURLSchemes ""
!insertmacro RemoveURLSchemes "un."

; UserPath puts the folder $R0 on the user's Path, with the app's command companion and commands, or takes it off when $R1 is "1", leaving the script's exit code in $R2: 3 when it added a folder that was not there, 4 when the folder is off Path, and anything else, empty without PowerShell, when it failed.
!macro UserPath un
Function ${un}UserPath
  Push $0
  StrCpy $R2 ""
  StrCpy $0 "$WINDIR\Sysnative\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $0 +2
  StrCpy $0 "$SYSDIR\WindowsPowerShell\v1.0\powershell.exe"
  IfFileExists $0 0 done
  System::Call 'Kernel32::SetEnvironmentVariable(t "NSGO_APP_DIR", t "$R0")'
  System::Call 'Kernel32::SetEnvironmentVariable(t "NSGO_PATH_REMOVE", t "$R1")'
  nsExec::Exec `"$0" -NoProfile -NonInteractive -ExecutionPolicy Bypass -Command "$$ErrorActionPreference = 'Stop'; $$d = $$env:NSGO_APP_DIR.TrimEnd('\\'); $$k = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment'); $$p = @(([string]$$k.GetValue('Path', '', 'DoNotExpandEnvironmentNames')) -split ';' | Where-Object { $$_ }); $$o = @($$p | Where-Object { $$_.TrimEnd('\\') -ne $$d }); if ($$env:NSGO_PATH_REMOVE) { if ($$o.Count -eq $$p.Count) { exit 4 }; $$p = $$o } else { if ($$o.Count -lt $$p.Count) { exit 0 }; $$p += $$d }; if ($$p.Count) { $$k.SetValue('Path', ($$p -join ';'), 'ExpandString') } else { $$k.DeleteValue('Path', $$false) }; if ($$env:NSGO_PATH_REMOVE) { exit 4 }; exit 3"`
  Pop $R2
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  done:
  Pop $0
FunctionEnd
!macroend
!insertmacro UserPath ""
!insertmacro UserPath "un."

; RemoveOwnedPath takes the folder the uninstall entry records an installer put on the user's Path off it, that folder alone, and forgets it once it is off; a removal that fails keeps the record, for the next upgrade or the uninstaller to try again. An installation from before the record has none, so its folder stays.
!macro RemoveOwnedPath un
Function ${un}RemoveOwnedPath
  Push $R0
  Push $R1
  Push $R2
  ReadRegStr $R0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NsgoPathAddedDir"
  StrCmp $R0 "" done
  StrCpy $R1 "1"
  Call ${un}UserPath
  StrCmp $R2 "4" 0 done
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NsgoPathAddedDir"
  done:
  Pop $R2
  Pop $R1
  Pop $R0
FunctionEnd
!macroend
!insertmacro RemoveOwnedPath ""
!insertmacro RemoveOwnedPath "un."

Section
  Call CloseApp
  SetOutPath "$INSTDIR"
  File /r /x "FrostMail.nsi" /x "frostmail-0.9.0-windows-x64-setup.exe" /x "FrostMail-setup.ico" /x "FrostMail-uninstall.ico" "*"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  ; An earlier version may have put the folder on Path.
  Call RemoveOwnedPath
  Call RemoveFileTypes
  Call RemoveURLSchemes
  CreateShortCut "$SMPROGRAMS\FrostMail.lnk" "$INSTDIR\FrostMail.exe"
  ; Give desktop toasts a COM activator CLSID on the Start menu shortcut.
  ExecWait '"$INSTDIR\FrostMail.exe" --nsgo-register-toast="$SMPROGRAMS\FrostMail.lnk"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "DisplayName" "FrostMail"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "DisplayVersion" "0.9.0"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "DisplayIcon" "$INSTDIR\FrostMail.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NoRepair" 1
  ; WinSparkle runs the installer with /S /RELAUNCH, once the app quit:
  ; the new version starts again.
  ${GetParameters} $0
  ClearErrors
  ${GetOptions} $0 "/RELAUNCH" $1
  ${IfNot} ${Errors}
    Exec '"$INSTDIR\FrostMail.exe"'
  ${EndIf}
SectionEnd

; The finish page offers it, so silent installs make none.
Function CreateDesktopShortcut
  SetOutPath "$INSTDIR"
  CreateShortCut "$DESKTOP\FrostMail.lnk" "$INSTDIR\FrostMail.exe"
FunctionEnd

Section "Uninstall"
  Call un.CloseApp
  ; An uninstaller left in a folder an upgrade moved from removes its own files alone: the registrations, shortcuts and Path are the installation's now in InstallLocation.
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "InstallLocation"
  StrCmp $0 $INSTDIR 0 files
  Call un.RemoveOwnedPath
  Call un.RemoveFileTypes
  Call un.RemoveURLSchemes
  ; Remove the per-user toast COM registration before deleting the EXE.
  ExecWait '"$INSTDIR\FrostMail.exe" --nsgo-unregister-toast'
  Delete "$SMPROGRAMS\FrostMail.lnk"
  Delete "$DESKTOP\FrostMail.lnk"
  ; A folder whose removal from Path failed keeps its record, for a later installation to take off.
  ReadRegStr $0 HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NsgoPathAddedDir"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail"
  StrCmp $0 "" files
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\org.frostyard.Frostmail" "NsgoPathAddedDir" $0
  files:
  ; What the installer put there alone, never the folder's other files.
  Delete "$INSTDIR\FrostMail.exe"
  Delete "$INSTDIR\THIRD_PARTY_NOTICES.txt"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd
