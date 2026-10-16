# Plan 0010: M7 Microsoft accounts and Windows

M7 makes Frostmail usable by the user's friend: Hotmail mail, contacts and
calendar, in Frostmail on Windows. It adds the `microsoft` account kind with
Microsoft sign-in (mail over IMAP and SMTP, contacts and calendars over
Microsoft Graph, [ADR-0039](../adr/0039-microsoft-accounts-over-imap-smtp-and-graph.md))
and a Windows build of maild, mailctl and the app with an installer
([ADR-0038](../adr/0038-windows-as-a-second-platform.md)). It runs after
[M5.5](0009-m5.5-edit-events-and-contacts.md) and before M6's Flatpak
packaging in the [roadmap](0002-roadmap.md). Microsoft comes first, on
Linux, where it can be tested against the user's Outlook.com account with
today's tools. Windows follows; Phase 4 can run alongside Phase 3.

## Decisions for M7

The user chose these on 2026-10-15:

- **Windows now, macOS later, not ruled out.** Platform code goes behind
  per-OS files, so a third platform does not reshape the second.
- **A personal account.** The friend's account is Hotmail, so tenant
  consent and SMTP AUTH policy are out of scope. Work and school accounts
  are Later.
- **Mail, contacts and calendar.** Contacts and calendars need Graph,
  since Outlook.com has no CardDAV or CalDAV. Microsoft To Do is Later;
  flagged mail stays the account's tasks.
- **The user's Outlook.com account is the test account.** It is barely
  used. Test mail and invitations go only among the user's own accounts
  (bjeffreyket@gmail.com, bketelsen@gmail.com, brianketelsen@icloud.com
  and the Outlook.com address), as in M5.5. Scrubbed recordings of it may
  be committed, as the throwaway Gmail's are.
- **Frostmail's Microsoft client** is registered (the user, 2026-10-15):
  client ID `8007f9b9-a6b6-4029-8295-3e6af287317b`, built into maild in
  Phase 2.
- **The user's Windows machines** are for installs, trials and checks that
  CI cannot make.

## Phase 1 — Probe Outlook.com (planner, small) — done

- The user registers Frostmail's public client in Microsoft Entra (setup
  below) and gives its client ID. Done 2026-10-15.
- `tools/msprobe` signs in through maild's loopback flow and reports,
  read-only except one message to the user's own address:
  - which scope forms get tokens that IMAP, SMTP and Graph accept, from
    one sign-in, and how refresh tokens rotate and expire;
  - IMAP's capabilities (CONDSTORE, QRESYNC, IDLE, MOVE, UIDPLUS,
    SPECIAL-USE, ESEARCH), its folders and their roles, connection limits,
    and how it reports flags, deletes and moves;
  - SMTP's submission with XOAUTH2, and whether Outlook.com saves a Sent
    copy itself;
  - Graph's calendars, `calendarView/delta`, a recurring series with an
    exception and a cancelled occurrence, attendees and responses,
    reminders, and change keys;
  - Graph's contact folders, contacts, photos, and whether contacts delta
    works for a personal account.
- **Done when:** the findings are recorded here, the profile's quirks are
  known, and an ADR (0040) decides how Graph's events and contacts are
  stored, synced, edited and undone.

**Findings (2026-10-15).** The probe signed in once as the user's
Outlook.com account and ran `imap`, `smtp`, `graph`, `writes` and `send`.
The user allowed the probe items and the one message. Everything the
probe made was deleted, and a last listing found none left.

- **Sign-in:**
  - One authorization at the `common` authority, with the resource-prefixed
    scopes (`https://outlook.office.com/IMAP.AccessAsUser.All`,
    `.../SMTP.Send`, `https://graph.microsoft.com/Calendars.ReadWrite`,
    `.../Contacts.ReadWrite`, `.../User.Read`) plus `offline_access`,
    consents to both audiences.
  - The code is redeemed with the Outlook scopes, and the refresh token is
    redeemed again for Graph's.
  - The refresh token rotates on every redemption (457–501 bytes), and
    access tokens last an hour.
  - The redirect `http://localhost:<port>` matches the registered
    `http://localhost`. The ID token's `email` claim names the account.
- **IMAP:**
  - `outlook.office365.com:993` and `imap-mail.outlook.com:993` are
    identical, with XOAUTH2 as the account's address.
  - Capabilities: `IMAP4rev1 IDLE MOVE UIDPLUS NAMESPACE CHILDREN LITERAL+
    SASL-IR UNSELECT ID AUTH=XOAUTH2`. There is no CONDSTORE, QRESYNC,
    SPECIAL-USE, ESEARCH, BINARY or LIST-STATUS.
  - LIST still marks `Sent`, `Drafts`, `Deleted` (`\Trash`) and `Junk`.
    `Archive` has no attribute.
  - Outlook's own folders show too: `Notes`, `Outbox`, and `Sync Issues`
    with three children.
  - `HIGHESTMODSEQ` is 0 everywhere, and INBOX's UIDs are sparse (1,808
    messages, `UIDNEXT` 13485).
  - It held six connections at once.
- **SMTP:** `smtp-mail.outlook.com:587` and `smtp.office365.com:587` take
  XOAUTH2 after STARTTLS, with `SIZE` 157286400. A message sent to the
  account itself was in `Sent` and INBOX within 5 seconds. Outlook.com
  saves the Sent copy itself (`SavesSent`).
- **Graph calendars:**
  - The account has `Calendar` (the default), `Birthday calendar` and
    `US Holidays` (read-only), and `Your family`, a group calendar that can
    be edited.
  - `/me/calendars/{id}/calendarView/delta` works on each of them over a
    window, and returns masters, singles, exceptions and bare occurrences.
    A deletion is `@removed` with only the ID, once per series.
  - Default IDs equal the immutable IDs.
  - A stale `If-Match` gets 412.
  - Zones are kept as written, IANA or Windows. All-day events run from
    midnight to midnight. Each event has one reminder.
  - Masters list `cancelledOccurrences` and `exceptionOccurrences` (v1.0).
    Renaming a master renamed its moved occurrence.
  - Filtering events by `seriesMasterId` is unsupported (501).
- **Graph contacts:**
  - The account has the default folder only.
  - `/me/contacts/delta` works for a personal account.
  - A photo is `PUT .../photo/$value`, and comes back as JPEG.
- **To Do:** 401 without its scope, as expected (Later).

The probe also left these for Phase 2:
- what `SELECT` (not `EXAMINE`) gives as `PERMANENTFLAGS`, which decides
  whether keywords and flag colors reach the server;
- whether Outlook.com's `\Recent` and sparse UIDs need anything;
- how a deleted occurrence alone shows in delta.

[ADR-0040](../adr/0040-graph-events-and-contacts-as-derived-sources.md)
records how Graph's events and contacts are stored.

## Phase 2 — Outlook mail (large)

- The `microsoft` account kind, which the API and the store already had,
  its provider profile, and discovery for Microsoft's domains. Done.
- `oauth`: the Microsoft endpoint with the built-in client ID, the
  `localhost` redirect, Outlook's scopes on every token request, and the
  refresh token stored on every rotation before it is used. Google's path
  does not change. An access token per audience comes with Graph in
  Phase 3. Done.
- The engine with Outlook's quirks: flags without CONDSTORE (the generic
  path), folder roles by attribute and name, no Sent append, and
  `NoKeywords`, since Outlook.com drops keywords and flag colors (found
  2026-10-15 on the probe's own message). Done.
- The account form signs in with Microsoft
  ([specs/settings-ui.md](../specs/settings-ui.md)). Done.
- Recordings of the user's Outlook.com account, scrubbed, replayed in
  `make check` (`TestReplayOutlook`). Done.
- **Evidence so far (2026-10-16):**
  - **Sign-in:** a dev maild with its own store signed the user's
    Outlook.com account in through `mailctl account connect --oauth`.
  - **Sync and verify:** it synced 30 days, and verify was clean once a
    pass had taken Deleted's three stand-ins.
  - **Recorded ops:** mailctl's flag, color, unread, move to Archive and
    back were recorded and replay as they ran. The color reached Outlook.com
    as `\Flagged` alone.
  - **Display copies:** the 37 display copies matched the whole messages
    after the Exchange CRLF fix ([sync.md](../design/sync.md#bodies)).
  - **Sending:** a message from the account to the user's Gmail arrived
    there. It also turned up two bugs, now fixed with tests:
    - maild appended a Sent copy, since only Gmail's kind skipped it, so
      Sent held two. The profile's `SavesSent` now decides.
    - Outlook.com rewrites the Message-ID, keeping the client's in
      `X-Microsoft-Original-Message-ID`, so a send interrupted after
      delivery would have gone out again. The profile's
      `OriginalMessageID` now finds it.
  - **Restart:** the dev maild restarted on its stored, rotated refresh
    token without asking to sign in.
  - **Still to do:** three days on the user's own maild, and a sign-out.
- **Done when:** on Linux, the user's Outlook.com account syncs, sends to
  their own addresses, survives a restart and a sign-out, and verifies
  clean for three days, with `make check` green.

## Phase 3 — Outlook contacts and calendars (large)

- `internal/graph`: a small Graph client (delta, paging, `$batch`
  if Phase 1 shows it is needed, throttling with `Retry-After`) and
  `internal/graphtest`, an in-process Graph fake for tests, as davx and
  davtest are for DAV.
- pimsync's Graph backend, per ADR-0040:
  - collections: calendars and contact folders, with names and colors;
  - reading: delta sync, or comparison by change key;
  - the store: events, instances, alarms, contacts and people, so the
    Calendar, People, invitations and reminders work unchanged;
  - writes: create, edit, delete and restore through `pim_ops`, with
    conflicts on change keys and the 412 re-application where it fits;
  - recurring edits in all three scopes, and invitations through
    Outlook's own scheduling (accept, tentative, decline; invitees on
    events the user organizes);
  - contact photos.
- Verify for Graph accounts, and recordings replayed in `make check`.
- Deleting an event through Graph moves it to Deleted Items, which IMAP
  shows as a "Retrieval using the IMAP4 protocol failed" message (found in
  Phase 2). Decide whether deletes use Graph's `permanentDelete`, given
  that undo restores by posting again.
- **Done when:** the user's Outlook.com calendar and contacts show in
  Frostmail, and edits from either side reach the other within a pass,
  with clean verifies. An invitation between the user's Outlook.com and
  Gmail accounts gets exactly one invitation, update and cancellation
  each.

## Phase 4 — maild and mailctl on Windows (medium)

- Per-OS files ([ADR-0038](../adr/0038-windows-as-a-second-platform.md)):
  - the named pipe with its DACL and client SID check;
  - Windows paths;
  - the DPAPI secret store;
  - embedded tzdata and the local zone's IANA name;
  - autostart, and maild's side of the notifier.
- Test helpers that run on Windows (`rpctest`, temp dirs, sockets).
- A Windows CI job: `go vet` and `go test ./...` on `windows-latest`. The
  Linux jobs also cross-compile for Windows.
- **Done when:** the Windows job passes the Go suite, and on one of the
  user's Windows machines maild syncs the Outlook.com and Gmail accounts,
  which `mailctl ls` and `mailctl show` read.

## Phase 5 — The app on Windows (large)

- The bridge over the named pipe, Windows paths in `paths.rs`, and the
  WebKit workarounds behind `cfg(target_os = "linux")`.
- `frostmail-notify.exe`: toasts under Frostmail's AppUserModelID, and a
  click that opens the message (single instance, as on Linux).
- The NSIS installer: per user, with maild, mailctl, the notifier, the
  Start Menu shortcut, the `Run` key, and an upgrade that keeps the data.
- The reader on WebView2: the zero-network hostile-mail suite, links
  ([ADR-0036](../adr/0036-reader-links-open-as-new-window-requests.md)),
  embedded images, and the window chrome and keyboard map, run by
  tauri-driver with msedgedriver in the Windows CI job.
- **Done when:** a clean install from the installer on one of the user's
  Windows machines reads, sends and edits all three of the user's test
  accounts. The hostile-mail suite makes zero requests on WebView2, and a
  reminder and a new-mail notification arrive with the app closed.

## Phase 6 — Trial and hand-off

- The user runs the Windows build for a week with Outlook.com (and Gmail
  or iCloud), with clean verifies.
- Then the friend installs it on his Hotmail account, with a short guide:
  the SmartScreen step, the Microsoft consent screen, and where logs are.
- **Done when:** the friend has read, sent and edited his calendar and
  contacts on Frostmail for a week without data loss.

## Setup for the user (Phase 1, done 2026-10-15)

Register Frostmail with Microsoft once. Its client ID ships in maild, so
nobody else has to:

1. Sign in at [entra.microsoft.com](https://entra.microsoft.com) with a
   Microsoft account. If it has no directory, a free Azure account
   creates one ("Default Directory"); registering an app costs nothing.
2. **App registrations → New registration:** name `Frostmail`; supported
   account types "Accounts in any organizational directory and personal
   Microsoft accounts"; redirect URI platform "Public client/native
   (mobile & desktop)" with `http://localhost`.
3. **Authentication:** "Allow public client flows" set to Yes. Add no
   client secret.
4. **API permissions** (delegated, Microsoft Graph): `offline_access`,
   `openid`, `email`, `User.Read`, `IMAP.AccessAsUser.All`, `SMTP.Send`,
   `Calendars.ReadWrite`, `Contacts.ReadWrite`. Personal accounts consent
   at sign-in, so these mainly document what Frostmail asks for.
5. Copy the **Application (client) ID** from Overview.

## Later / ideas

- macOS (ADR-0038 left the seams for it).
- Hiding Outlook's own folders (Outbox, Sync Issues and its children),
  which Exchange's IMAP lists like any other.
- Work and school Microsoft 365 accounts: tenant admin consent, SMTP AUTH
  or Graph `sendMail`, publisher verification.
- Microsoft To Do lists and tasks over Graph.
- Signed installers and automatic updates.
- Windows on arm64.

## Open questions

- **Reaching a Windows machine from the planner's side:** OpenSSH on one
  of the user's machines, or CI plus the user running installers. By
  Phase 4.

## References

- Implements: [ADR-0038](../adr/0038-windows-as-a-second-platform.md),
  [ADR-0039](../adr/0039-microsoft-accounts-over-imap-smtp-and-graph.md),
  [ADR-0040](../adr/0040-graph-events-and-contacts-as-derived-sources.md)
- Shapes: [design/accounts.md](../design/accounts.md),
  [design/pim.md](../design/pim.md),
  [design/sync.md](../design/sync.md),
  [design/desktop.md](../design/desktop.md),
  [design/testing.md](../design/testing.md)
