# Accounts, sign-in and providers

Living document. Rationale: [ADR-0011](../adr/0011-sign-in-and-credentials.md)
(sign-in, secrets), [ADR-0012](../adr/0012-gmail-labels-as-memberships.md)
(Gmail). Contracts: [specs/rpc-api.md](../specs/rpc-api.md) (`account.*`,
`oauth.*`).

## Provider profiles

`internal/providers` holds one profile per account kind:

| Kind | IMAP | SMTP | Sign-in | Quirks |
| --- | --- | --- | --- | --- |
| `gmail` | imap.gmail.com:993 TLS | smtp.gmail.com:465 TLS | `oauth2` or `password` (app password) | the Gmail sync path (ADR-0012); no Sent append; no QRESYNC; at most 15 connections |
| `icloud` | imap.mail.me.com:993 TLS | smtp.mail.me.com:587 STARTTLS | `password` (app-specific) | `ENABLE` gets no `ENABLED` reply (treat as enabled only if `CONDSTORE` answers); QRESYNC off; LIST marks "Sent Messages" and "Deleted Messages" with `\Sent` and `\Trash` though SPECIAL-USE is not offered, so other clients' "Sent Items" and "Trash" stay plain folders; EXAMINE answers `[READ-WRITE]`; empty folders report HIGHESTMODSEQ 0; the IMAP user name is the address |
| `microsoft` | outlook.office365.com:993 TLS | smtp-mail.outlook.com:587 STARTTLS | `oauth2` with Frostmail's own client | Outlook.com and Hotmail ([ADR-0039](../adr/0039-microsoft-accounts-over-imap-smtp-and-graph.md)): no Sent append; no CONDSTORE or SPECIAL-USE, but LIST marks Sent, Drafts, Deleted (`\Trash`) and Junk, and Archive goes by its name; keeps no keywords (`NoKeywords`), so flag colors stay local; SMTP gives a sent message a Message-ID of its own and keeps the client's in `X-Microsoft-Original-Message-ID` (`OriginalMessageID`); Outlook's own folders (Notes, Outbox, Sync Issues) are plain folders |
| `imap` | discovered or entered | discovered or entered | `password` | none |

The sync engine reads quirks from the profile, never from host names. A
profile's servers fill `account.create` when the request leaves them out.

## Discovery

For a generic address `account.discover {email}` tries, in order, and
returns the first complete answer with where it came from:

1. a provider profile, by domain (gmail.com, googlemail.com, icloud.com,
   me.com, mac.com);
2. Mozilla autoconfig at `https://autoconfig.<domain>/mail/config-v1.1.xml`
   and `https://<domain>/.well-known/autoconfig/mail/config-v1.1.xml`;
3. the Thunderbird ISPDB (`https://autoconfig.thunderbird.net/v1.1/<domain>`);
4. RFC 6186 SRV records (`_imaps._tcp`, `_imap._tcp`, `_submissions._tcp`,
   `_submission._tcp`);
5. nothing: the user enters the servers.

Discovery fetches only over HTTPS, follows no redirects to other hosts,
caps responses at 64 KB, and never sends the password anywhere.
`%EMAILADDRESS%` and `%EMAILLOCALPART%` placeholders are expanded.

## Sign-in

- **Passwords** go to `account.setPassword` and the secret store, as in M1.
- **OAuth** (Gmail): the user's client is set once with `oauth.setClient
  {provider, clientId, clientSecret}`. `account.authorize {id}` returns an
  authorization URL; maild listens on `127.0.0.1:<random port>` for the
  redirect (one request, matching `state`, 5-minute limit), exchanges the
  code with the PKCE verifier, stores the refresh token
  (`account/<id>/refresh-token`) and restarts the account. The app opens
  the URL with `open_link` and shows "Waiting for Google…" until
  `account.changed` reports the account signed in.
- **OAuth** (Microsoft): maild has Frostmail's public client built in
  (`oauth.MicrosoftClientID`, no secret); `oauth.setClient` can replace
  it. The sign-in runs at the `common` authority with the scopes
  `https://outlook.office.com/IMAP.AccessAsUser.All`, `.../SMTP.Send` and
  `offline_access` and `prompt=select_account`. The redirect is
  `http://localhost:<port>`, as the client is registered, and maild
  listens on that port over IPv4 and IPv6. Every token request names
  Outlook's scopes, since Microsoft grants a token per audience (Graph's
  joins in Phase 3 of [plan 0010](../plans/0010-m7-microsoft-and-windows.md)).
  Microsoft rotates the refresh token on every use; maild stores the new
  one before it uses the access token.
- **Tokens:** `internal/oauth` keeps access tokens in memory, refreshes
  them when less than 5 minutes remain, and once after `AUTHENTICATE`
  fails. `invalid_grant` makes the account `unauthorized` with "Sign in
  again" in the sidebar and settings.
- **A sign-in also finishes from a pasted address** when the browser
  cannot reach maild's loopback, as behind lodge (ADR-0043).
  `account.finishSignIn {url}` takes the address the provider sent the
  browser to. maild reads its `state` and finds the waiting sign-in by a
  constant-time comparison. The address's scheme, host, port and path
  must be the waiting redirect's; a path of "" and "/" are equal. Then
  maild does what the loopback listener does: it refuses the sign-in when
  the query holds the provider's `error`, or exchanges the code with the
  same redirect and PKCE verifier, stores the refresh token, clears "Sign
  in again", reconnects the account and emits `account.changed`. The
  listener then closes. A sign-in finishes once, by whichever way comes
  first; an unknown, finished or expired `state` is `invalidParams` and
  the user starts the sign-in again. maild logs no code and no address.
  Only `local` and `session` callers may use the method, never an agent.
  The clients show a field while a sign-in waits
  ([settings-ui.md](../specs/settings-ui.md)); `mailctl account
  finish-signin URL` does the same.
- **A refused login is not always the credential.** A `NO` with the code
  `UNAVAILABLE` (RFC 5530), or Outlook's "User is authenticated but not
  connected.", means the server took the token but cannot open the
  mailbox now. Outlook answers it for minutes at a time, often after it
  drops an IDLE connection, while the mailbox's backend moves. Sync treats
  it as `offline` and retries with backoff (`imapx.ErrUnavailable`); only
  another `NO` marks the account. A login that succeeds clears the mark.
- **XOAUTH2** is the SASL mechanism for both IMAP (`AUTHENTICATE XOAUTH2`)
  and SMTP (`AUTH XOAUTH2`); a failure's base64 JSON details are decoded
  into the error.

## Secrets

- `secrets.Store` gains `SecretService`: D-Bus `org.freedesktop.secrets`,
  default collection (`/org/freedesktop/secrets/aliases/default`), items
  labeled `Frostmail: <key>` with attributes `application=frostmail`,
  `key=<key>`, plain session (the bus is local). A locked collection is
  unlocked with the service's prompt.
- `FROSTMAIL_SECRETS` chooses the store, and an unknown value is an error:
  - unset: the Secret Service when the session bus offers it, else the
    file store with a warning;
  - `file`: the plain file store, with a warning that a server seals it
    (tests, CI, development);
  - `sealed`: the sealed file store, below (Linux only; on Windows it is an
    error).
- Migration: on start with the Secret Service, every key of an existing
  `secrets.json` is copied in, verified by reading it back, and the file is
  removed.
- On Windows (ADR-0038) the store is `secrets.dpapi` beside the database:
  the same JSON object, sealed for the user with `CryptProtectData`, which
  only the user's Windows sign-in can unseal and which has no size cap,
  unlike Credential Manager's 2,560 bytes. `FROSTMAIL_SECRETS=file` keeps
  the plain file there too, and a plain `secrets.json` left over is moved
  in on start, as above. Windows has no file modes, so the plain file's
  0600 check is Unix-only.
- Keys: `account/<id>/password`, `account/<id>/refresh-token`,
  `oauth/<provider>/client-secret`. Deleting an account deletes its keys.
- **The sealed store** is for a server (ADR-0041, and
  [ADR-0044](../adr/0044-seal-the-servers-secrets-with-a-systemd-credential.md),
  accepted 2026-10-24, built): `FROSTMAIL_SECRETS=sealed`, which
  `frostmail.service` sets.
  - `internal/secrets/sealed.go`: the file store's JSON object, sealed
    with AES-256-GCM as `secrets.sealed` beside the database, mode 0600,
    written like `secrets.json` (a temporary file, synced, renamed). The
    file is the magic `FMSEAL01`, a fresh random 12-byte nonce for each
    write, then the ciphertext and its tag; the magic is also the
    additional data.
  - The key is 32 raw bytes, the systemd credential `frostmail-secrets-key`
    (`LoadCredentialEncrypted=` in the unit), which maild reads from
    `$CREDENTIALS_DIRECTORY`.
  - **No silent fallback.** maild refuses to start (it exits with the
    error) when the credentials directory or the key is missing, the key is
    not 32 bytes, or the key does not open an existing `secrets.sealed`.
    The error names the credential and the file, never a secret or the key,
    and a plain `secrets.json` stays as it was. It never falls back to a
    plain file or to the Secret Service. A missing `secrets.sealed` is an
    empty store, the first start.
  - **Migration.** On start, a plain `secrets.json` moves in once: each key
    is copied, read back, and the plain file removed, as for the Secret
    Service. maild reads the sealed file first, so a plain file never moves
    into a file that nothing opens.
  - `mailctl export` with `sealed` reads `secrets.sealed`, then a plain
    leftover, and never writes. It needs `$CREDENTIALS_DIRECTORY` to hold
    the key, so it runs as a service with the unit's
    `LoadCredentialEncrypted=`; it fails without the key.
  - **The key is not in a backup of the data directory.** A restore
    onto another host cannot open `secrets.sealed`: move it aside, and each
    account shows "Sign in again".
  - `scripts/seal-mail-secrets.sh` (as root, on lodge's host) makes the
    key and lets maild seal an existing `secrets.json`: dry run by default,
    a backup of the plain file and a manifest, `--rollback`. The move
    script (`scripts/move-mail-to-lodge.sh`) accepts `file` and `sealed`.
  - `FROSTMAIL_SECRETS=file` stays for a server without systemd
    credentials, and for CI and development.
- **Moving to another maild.** `mailctl export DIR`, with maild stopped,
  reads every account's keys and the OAuth clients' secrets from whichever
  store maild uses, without changing it, and writes them to
  `DIR/secrets.json` (0600) beside a copy of the database and the blobs.
  `scripts/move-mail-to-lodge.sh` uses it to move the desktop's mail to
  lodge's host ([plan 0011](../plans/0011-m8-mail-in-lodge.md), Phase 7).

## Read-only accounts

An account can be read-only (`account.readOnly`, set at creation or with
`account.update`): maild selects mailboxes with `EXAMINE`, never replays
offline operations or appends, and refuses `message.setFlags`,
`message.move`, `message.delete`, `draft.send` and server draft copies for
it with `conflict` ("account is read-only"). Drafts stay local. The app
does not mark messages read on such accounts and shows "Read-only" in the
toolbar subtitle. New accounts start writable; the add-account form's
Read only checkbox makes one read-only (it was on by default during M4's
trial).

## Sync window

An account keeps the mail of its last `syncDays` days (`account.syncDays`,
0 for everything, set at creation or with `account.update`;
[ADR-0016](../adr/0016-sync-a-window-of-recent-mail.md)): a big account
syncs in an hour rather than a day and stays within Gmail's 2,500 MB daily
IMAP download limit. `mailctl account set ID --sync-days N` changes it (and
`--read-only`/`--read-write`, `--notify`/`--no-notify`); `mailctl account
connect --sync-days N` sets it at creation. Verify compares the window the
folder's last pass kept.

## Safety check

`mailctl verify ACCOUNT` (`account.verify`) compares the store with the
server: for each synced folder it lists UIDs on the server and locally and
reports messages missing on either side, flag differences, and, for Gmail,
label differences, without changing anything. During the 7-day trial it
runs daily; any difference that is not explained by mail arriving during
the check is a bug.

## Recordings and replay

`MAILD_IMAP_TRACE=<dir>` writes each sync and IDLE connection's exchange
to a file. `tools/imaprec` turns a trace into a replay script: credentials
are already redacted, every word of the mail becomes a stable fake, and big
sessions are trimmed to a few messages per mailbox; the result is checked
into `internal/mailsync/testdata/replay/`. `internal/imapx/replay` is a
server that plays a script back and fails the test on any command the
script does not expect. Replay tests cover the Gmail, iCloud and
Outlook.com paths no local server can imitate; [testing.md](testing.md#recorded-sessions) has
the details.

## Setting up a personal Google client

Once per user, until M6 ships a verified client:

1. In the Google Cloud console, create a project and enable the Gmail API.
2. Configure the OAuth consent screen (Google Auth Platform): audience
   External; add the scope `https://mail.google.com/`; **publish the app**
   so its status is "In production" (in "Testing", refresh tokens expire
   after 7 days). It stays unverified: Google shows a warning screen at
   sign-in, which you accept for your own client.
3. Create an OAuth client ID of type **Desktop app**; keep its client ID and
   client secret.
4. Give them to Frostmail: Settings → Accounts → Google client, or
   `mailctl oauth set-client google --client-id … --client-secret …`.

App passwords (fallback): turn on 2-Step Verification, then create an app
password at myaccount.google.com/apppasswords. iCloud: create an
app-specific password at account.apple.com → Sign-In and Security.
