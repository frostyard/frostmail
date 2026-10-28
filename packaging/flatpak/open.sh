#!/bin/sh
# Frostmail's notification click in the Flatpak (docs/design/desktop.md,
# Notifications; docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md).
# The manifest installs it as /app/libexec/frostmail-open, and the launcher
# names it in FROSTMAIL_APP for the maild of a new sandbox, so that a click on
# a notification opens the window there.
#
#   frostmail-open [ARGS]    Run the app with ARGS, as a start does:
#                            --open-message <id> for a clicked message, and no
#                            argument for a group's notification.
#
# A window that maild ran itself would be a child of maild, in maild's
# sandbox, and `flatpak ps` would show no instance of its own. So the click
# goes to the Flatpak portal, which starts the command in a new sandbox of
# this app: a normal start there makes sure the latest maild runs and runs the
# window, and nothing runs in this sandbox.
#
# --latest-version opens the newest installed deployment, never the one of the
# maild that sent the notification: an old notification's click cannot bring
# back an old build, as the launcher's own starts cannot (ADR-0046).

set -u

if command -v flatpak-spawn >/dev/null 2>&1; then
    exec flatpak-spawn --latest-version frostmail "$@"
fi

# Without flatpak-spawn there is no Flatpak portal: maild is a host one (or a
# test's), and the app of this sandbox is the right one.
exec frostmail "$@"
