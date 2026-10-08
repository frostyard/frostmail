# 0011 — maild signs in with the user's own OAuth client and keeps secrets in the Secret Service

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

M4 makes Frostmail the user's only mail client for a week, on a personal
Gmail account and an iCloud account. Gmail takes OAuth 2.0 (XOAUTH2) or an
app password; iCloud takes an app-specific password. Google's
`https://mail.google.com/` scope is restricted: a public client needs
Google's verification (planned for M6), while an unverified client that its
owner publishes ("In production") works for up to 100 users behind one
warning screen. In "Testing" status Google expires refresh tokens after 7
days, which would break a 7-day trial. Google does not allow mail scopes in
its device-code flow. maild keeps syncing and sending while the app is
closed ([ADR-0002](0002-daemon-and-thin-clients.md)), so it must refresh
tokens itself. Secrets live in a plain 0600 JSON file since M1.

## Decision

- **Two kinds of sign-in:** `password` (including Google app passwords and
  iCloud app-specific passwords) and `oauth2` (SASL XOAUTH2 for IMAP and
  SMTP). Gmail accounts may use either; iCloud and generic accounts use
  passwords.
- **Bring your own client until M6.** Frostmail ships no OAuth client. The
  user registers one (a Google Cloud "Desktop app" client, published, not
  verified) and gives it to maild (`oauth.setClient`): the client ID is
  stored in the database, the client secret (not secret for desktop clients,
  but treated as one) in the secret store. maild knows each provider's
  endpoints and scopes; Google first, Microsoft as a second entry later.
- **maild runs the authorization.** `account.authorize` starts an
  authorization-code flow with PKCE (S256): maild listens on a random
  loopback port (`http://127.0.0.1:<port>/`) for that one redirect, checks
  `state`, and gives up after 5 minutes; it returns the authorization URL,
  which the app opens in the system browser (mailctl prints it). maild
  exchanges the code, stores the refresh token in the secret store, and
  restarts the account.
- **Tokens stay in maild.** Access tokens live in memory and are refreshed
  when less than 5 minutes remain, and once after an authentication failure.
  A refresh refused with `invalid_grant` marks the account unauthorized
  ("Sign in again"); nothing retries it until the user signs in.
- **The Secret Service holds secrets.** maild talks to
  `org.freedesktop.secrets` over D-Bus (pure Go, godbus): items in the
  default collection, labeled `Frostmail: <key>` with attributes
  `application=frostmail` and `key=<key>`; a locked collection is unlocked
  through the service's own prompt. Without a Secret Service (CI, the nsl
  machine) maild uses the file store and logs a warning. When the Secret
  Service is available and the file store holds secrets, maild moves them
  over once and deletes the file.
- **Provider profiles.** maild has a profile per account kind (`gmail`,
  `icloud`, `imap`): server hosts, ports, TLS, allowed sign-in kinds, and
  quirks the sync engine honors. Creating a Gmail or iCloud account needs
  only the address and a sign-in; other accounts use discovery (Mozilla
  autoconfig, the ISPDB, RFC 6186 SRV records) and can be edited.

## Consequences

- Each user registers an OAuth client until the verified one ships in M6;
  the setup is documented in [design/accounts.md](../design/accounts.md).
- Refresh tokens never pass through the webview, and maild keeps working
  with the app closed.
- The loopback listener exists only during a sign-in, on 127.0.0.1, for one
  request with the expected `state`.
- Moving to the Secret Service removes the plain-text file on desktops; CI
  and the dev machine keep the file store.
- Microsoft becomes a profile and a test, not a design change.

## Alternatives considered

- **OAuth in the app (Rust):** the refresh token would still have to reach
  maild, which refreshes while the app is closed.
- **Device-code flow:** Google does not grant mail scopes to it.
- **libsecret through cgo:** maild is built with `CGO_ENABLED=0`; the D-Bus
  API is small enough to call directly.
- **App passwords only:** they work for personal Gmail today, but Google
  has been narrowing them, and Microsoft accounts need OAuth anyway.
