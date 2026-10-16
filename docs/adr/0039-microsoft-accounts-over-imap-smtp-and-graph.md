# 0039 — Microsoft accounts: mail over IMAP and SMTP, contacts and calendars over Graph

- **Status:** Accepted
- **Date:** 2026-10-15

## Context

The user's friend needs Hotmail (a personal Microsoft account) with mail,
contacts and calendar, on Windows (ADR-0038). The user has an Outlook.com
account to test with. Microsoft accounts were deferred in M4 until an
account needed them. Facts checked 2026-10-15:

- **Mail:** Outlook.com serves IMAP and SMTP to third-party clients with
  OAuth 2.0 (SASL XOAUTH2). Developers report that the sign-in is picky
  about the scopes' exact form
  (`https://outlook.office.com/IMAP.AccessAsUser.All` and `SMTP.Send`).
- **Contacts and calendars:** Microsoft offers no CardDAV or CalDAV for
  Outlook.com or Microsoft 365. Microsoft Graph serves both to personal
  accounts. Calendar sync has a documented delta query for personal
  accounts (`/me/calendarView/delta`). Contacts delta for personal
  accounts is not documented either way. Exchange Web Services is being
  retired, and Exchange ActiveSync is a licensed binary protocol.
- **Sign-in:** an app must be registered in a Microsoft Entra directory.
  A desktop app is a public client: a client ID, a loopback redirect,
  PKCE, and no secret. Personal accounts can consent to an unverified app
  themselves. Work and school tenants often require an administrator's
  consent for IMAP. An access token has one audience (Outlook's IMAP and
  SMTP, or Graph), but a refresh token belongs to the user and client and
  can be redeemed for each audience the user consented to. Refresh tokens
  rotate on every use.
- Frostmail's sign-in (ADR-0011) already runs PKCE through a loopback
  listener in maild, keeps refresh tokens in the secret store, and was
  written so that Microsoft would be "a profile and a test". Its PIM sync
  (ADR-0017) speaks CardDAV, CalDAV and the Google Tasks API, and keeps
  each object as the server sent it (ADR-0018).

## Decision

- **An `outlook` account kind** with a provider profile: Outlook.com's IMAP
  and SMTP hosts, OAuth only, `hotmail.com`, `outlook.com`, `live.com` and
  `msn.com` addresses, and whatever quirks Phase 1's probe finds. Mail
  syncs over IMAP and sends over SMTP with XOAUTH2, through the same
  engine as every other account.
- **Frostmail's own Microsoft client.** The user registers one public
  client (any Entra directory and personal accounts, a
  `http://localhost` redirect, no secret). Its client ID is built into
  maild. A public client's ID is not a secret, so nobody else registers
  anything, unlike Google's bring-your-own client (ADR-0011).
  `oauth.setClient` can still override it.
- **One sign-in, a token per audience.** The authorization asks for the
  IMAP, SMTP and Graph scopes of the services turned on, plus
  `offline_access`, at the `common` authority. maild keeps one refresh
  token per account, stores each rotated one before using it, and caches
  an access token per audience: Outlook for IMAP and SMTP, Graph for
  contacts and calendars.
- **Contacts and calendars over Microsoft Graph.** pimsync gains a Graph
  backend beside DAV and Google Tasks: Graph calendars and contact
  folders are collections. Sync uses delta queries where Graph offers
  them, and compares by change key where it does not. Writes are queued
  `pim_ops`, as for every other service. How Graph's JSON maps onto
  stored objects, recurrence, invitations and undo is decided in its own
  ADR after Phase 1's probe.
- **Microsoft To Do stays Later.** For this account, flagged mail is its
  tasks list, as on iCloud.
- **Work and school accounts are not a goal of this milestone.** The
  client accepts them, but tenant consent and SMTP policy are untested.

## Consequences

- The friend signs in with his Microsoft account and approves one consent
  screen, which names Frostmail as unverified. There is no registration
  step for him.
- Mail on Microsoft accounts gets every engine feature at once: rules,
  smart mailboxes, search, undo send, display copies.
- A Graph backend is new code beside the DAV one, with its own client,
  fake server and recordings, as davx and davtest were.
- Graph's JSON is not iCalendar or vCard, so verify, undo and the editor
  must work through a mapping, and its fidelity has to be tested.
- Refresh-token rotation means a token written to the secret store must
  never be lost: a crash between redeeming and storing would sign the
  account out.
- Publisher verification, which work tenants may require, needs a
  Microsoft partner account. That is Later.

## Alternatives considered

- **Graph for mail too:** one API for everything, but Frostmail's mail
  engine is IMAP. Rewriting sync for one provider would split every
  feature in two.
- **Exchange ActiveSync:** one protocol for mail, contacts and calendars,
  but binary, licensed, and a second mail engine.
- **Exchange Web Services:** being retired.
- **Bring your own client, as for Google:** the friend would need an Entra
  directory and a registration. A public client's ID can ship.

## References

- Shapes: [design/accounts.md](../design/accounts.md),
  [design/pim.md](../design/pim.md),
  [design/sync.md](../design/sync.md),
  [plans/0010](../plans/0010-m7-microsoft-and-windows.md)
- Builds on: [ADR-0011](0011-sign-in-and-credentials.md),
  [ADR-0017](0017-sync-contacts-calendars-and-tasks.md),
  [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md),
  [ADR-0035](0035-invite-through-the-providers-scheduling.md),
  [ADR-0038](0038-windows-as-a-second-platform.md)
