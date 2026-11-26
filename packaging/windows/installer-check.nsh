; The names of a check build of the Windows installer
; (FROSTMAIL_INSTALLER_CHECK=1 scripts/windows-installer.sh;
; docs/design/desktop.md, Windows). tools/nsgoinstaller writes this file
; into the script ahead of installer.nsh, whose names it replaces, so
; that an install of the check leaves a real install on the same machine
; alone: its toast scheme, the Tauri install it would take over, and the
; folder it takes for the mail's are the check's own. FM_CHECK leaves out the
; 2 steps that have no name to change: maild's -autostart on, and the
; removal of the Run key's Frostmail value. It starts the check's maild only
; when the installer's environment names a pipe and a data folder for it
; (FROSTMAIL_SOCKET, FROSTMAIL_DATA_DIR).
;
; The script gives the check its name, its program's name and its
; AppUserModelID (FM_NAME, FM_EXE, FM_APP_ID), with NSGo's -name and -id.

!define FM_CHECK
!define FM_TOAST_SCHEME "frostmail-toast-check"
!define FM_TAURI_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Frostmail Check"
!define FM_TAURI_VENDOR_KEY "Software\frostyard-check"
!define FM_DATA_ROOT "$LOCALAPPDATA\Frostmail Check"
