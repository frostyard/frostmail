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
- **Tokens:** `internal/oauth` keeps access tokens in memory, refreshes
  them when less than 5 minutes remain, and once after `AUTHENTICATE`
  fails. `invalid_grant` makes the account `unauthorized` with "Sign in
  again" in the sidebar and settings.
- **XOAUTH2** is the SASL mechanism for both IMAP (`AUTHENTICATE XOAUTH2`)
  and SMTP (`AUTH XOAUTH2`); a failure's base64 JSON details are decoded
  into the error.

## Secrets

- `secrets.Store` gains `SecretService`: D-Bus `org.freedesktop.secrets`,
  default collection (`/org/freedesktop/secrets/aliases/default`), items
  labeled `Frostmail: <key>` with attributes `application=frostmail`,
  `key=<key>`, plain session (the bus is local). A locked collection is
  unlocked with the service's prompt.
- maild picks the Secret Service when the session bus offers it, else the
  file store with a warning. `FROSTMAIL_SECRETS=file` forces the file store
  (tests, CI).
- Migration: on start with the Secret Service, every key of an existing
  `secrets.json` is copied in, verified by reading it back, and the file is
  removed.
- Keys: `account/<id>/password`, `account/<id>/refresh-token`,
  `oauth/<provider>/client-secret`. Deleting an account deletes its keys.

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
script does not expect. Replay tests cover the Gmail and iCloud paths no
local server can imitate; [testing.md](testing.md#recorded-sessions) has
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
