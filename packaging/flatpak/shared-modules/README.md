# Flathub shared modules, vendored

These files are copies of Flathub's shared modules, from
<https://github.com/flathub/shared-modules> at commit
`c91a81101cf9bcf9fc8132e8c3ebed741ad8bae6` (2026-11-13), unchanged, in the
same folders, so that their relative paths still work:

- `libayatana-appindicator/`: `libayatana-appindicator-gtk3.json` and its
  2 patches. It builds libdbusmenu-gtk3, ayatana-ido, libayatana-indicator
  and libayatana-appindicator3, which the GNOME runtime does not have. MyGo
  opens `libayatana-appindicator3.so.1` for the tray icon on Linux
  ([ADR-0060](../../../docs/adr/0060-close-to-the-tray-is-an-opt-in-setting.md)).
- `intltool/`: `intltool-0.51.json` and its patch, which libdbusmenu's
  build needs.

The manifest names `libayatana-appindicator-gtk3.json` as its first module.
Each source in these files is pinned: an archive by its sha256, a git
source by its commit.

A copy, not a git submodule: CI's checkout and the Flatpak's `dir` source
need nothing more. To update, copy the same files from a newer commit of
flathub/shared-modules, and change the commit above.
