# Security policy

Frostmail reads and sends the owner's mail, and keeps the passwords and
OAuth tokens of their accounts. Treat any report that touches account
secrets, the rendering of hostile mail, remote content, the lodge socket or
the scopes of `/mcp/mail` with care.

## Supported versions

Frostmail is pre-release. Fixes go to `main` and ship in the next release.
Only the latest release (the Flatpak `org.frostyard.Frostmail` and the
`frostyard-maild` package) gets fixes; there are no backports to older
releases.

## Reporting a vulnerability

Report it privately, through GitHub's private vulnerability reporting for
this repository: <https://github.com/frostyard/frostmail/security>
("Report a vulnerability").

Do not open a public issue for a vulnerability. Never post real mail,
passwords, tokens, the secrets file or logs that hold them.

## What to include

- the version (`maild -version`, or the Flatpak's version) or the commit;
- where it runs: the desktop Flatpak, maild as lodge's `mail` app, or a
  development build;
- the impact, and what you expected instead;
- the steps to reproduce it, or a made-up message that shows it.
