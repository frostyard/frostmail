# 0043 — Finish an OAuth sign-in with a pasted address when the browser cannot reach maild

- **Status:** Accepted (Brian, 2026-10-21), amended
- **Date:** 2026-10-21

## Context

maild signs an OAuth account in with the authorization-code flow, PKCE and
a loopback redirect ([ADR-0011](0011-sign-in-and-credentials.md),
[ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md)). For the
one sign-in, maild listens on a random port of its own loopback, and the
provider sends the browser to `http://127.0.0.1:<port>/` (Google) or
`http://localhost:<port>` (Microsoft).

Since [ADR-0041](0041-maild-runs-as-a-lodge-app.md), the owner's maild
runs on lodge's host and the browser runs on another tailnet device. The
browser cannot reach maild's loopback. ADR-0041 and
[plan 0011](../plans/0011-m8-mail-in-lodge.md) leave this as an open
question, to answer before Phase 7. On 2026-10-21 it happened: the owner
signed account 7 (Outlook.com) in from lodge, Microsoft sent his browser
to `http://localhost:39221/?code=…&state=…`, and the page did not load.
An operator finished the sign-in by hand with `curl` inside the VM.

Facts checked on 2026-10-21:

- **Microsoft Entra:**
  - A redirect URI is `https` anywhere, or `http` on localhost only. The
    port of a localhost URI is ignored when it is matched.
  - An app that signs in personal accounts takes at most 100 redirect URIs
    and no wildcards.
  - The device-code flow works for public clients when the registration
    allows public client flows. Many work and school tenants block it by
    Conditional Access, because it is used for phishing.
- **Google:**
  - A "Desktop app" client takes only loopback redirects. The out-of-band
    flow (copy a code from Google's page) was retired in 2022.
  - A "Web application" client takes `https` redirect URIs that the
    developer registers, and sends its secret with the code.
  - The device-code flow ("TVs and Limited Input devices") allows only
    OpenID, Drive file and YouTube scopes. `https://mail.google.com/` is
    not one of them.
  - Frostmail has no Google client of its own: each user registers one
    (ADR-0011). The owner's is a Desktop app.
- **The browser keeps the address.** When the redirect to a loopback port
  fails, the browser shows an error page, and its address bar still holds
  the whole redirect: `code` and `state` in the query.

## Decision

- **The redirect stays loopback, for every provider.** maild listens on
  its loopback for the one sign-in, as ADR-0011 says. No provider client
  registers an address of lodge or of any other host.
- **A sign-in also finishes from a pasted address.** A new method,
  `account.finishSignIn {url}`, takes the address the provider sent the
  browser to. It admits `local` and `session` callers, never `agent`.
  - maild reads `state`, `code` and `error` from the address's query. It
    finds the waiting sign-in by `state`, compared in constant time, and
    the address must have the waiting sign-in's redirect as its origin.
  - It finishes the sign-in as the loopback listener does: it exchanges
    the code with the same redirect URI and the PKCE verifier, stores the
    refresh token, clears "Sign in again", and reconnects the account.
    The listener then closes.
  - A sign-in finishes once, by whichever way comes first. An unknown,
    finished or expired `state` is `invalidParams`, and the user starts the
    sign-in again. maild logs no code and no address.
- **Clients off maild's machine show the field.** While a sign-in that
  Settings started waits, the web build and the desktop app on a remote
  maild show: "If your browser cannot open the page after you sign in,
  copy that page's address and paste it here", a field, and **Finish
  Sign-In**. The desktop app on its own maild keeps today's flow, with no
  field.
- **mailctl** gains `mailctl account finish-signin URL` for the same
  method.

## Consequences

- A sign-in from lodge works on any tailnet device with one paste, for
  Google and Microsoft, with the clients registered today. No provider
  registration changes, and the existing grants stay.
- Phase 7's sign-in question is answered.
- The paste is a manual step, and it is clumsy on a phone: the user must
  copy the address of an error page.
- The code passes through the browser's address bar and the field. It is
  single-use, short-lived and useless without the PKCE verifier, which
  never leaves maild.
- maild finds a waiting sign-in by `state`, not only by account. The
  loopback listener stays, for the desktop app on its own maild and for
  Windows (ADR-0038), where the redirect still reaches maild by itself.
- `account.finishSignIn` is additive in protocol 1.

## Alternatives considered

- **A redirect through lodge's own address** (`https://LODGE/mail/signin`,
  mail's route in lodge's shell, which finishes the sign-in with no manual
  step): rejected by the owner, who wants no lodge-specific address in the
  provider registrations. Each lodge host would also be one more
  registration in each provider's client, and Google would need a Web
  client and a new sign-in for every Gmail account.
- **The device-code flow:** Google does not allow Gmail's scope in it, so
  it would serve Microsoft only, and tenants often block it. The user also
  types a code, which is a manual step too.
- **A relay page at one public address,** which sends the browser on to
  the host named in `state` (Microsoft's documented answer to the redirect
  limit): the code would pass through a page outside the tailnet, and the
  page could become an open redirector.
- **Sign in on the desktop app and hand the grant to the server:** it needs
  the desktop app, a second maild and a way to move a refresh token
  between machines. lodge exists so that a browser is enough.
- **A tunnel to maild's loopback for each sign-in:** a manual step on every
  device, and impossible from a phone.

## References

- Shapes: [design/accounts.md](../design/accounts.md#sign-in), [specs/settings-ui.md](../specs/settings-ui.md), [specs/rpc-api.md](../specs/rpc-api.md), [plan 0011](../plans/0011-m8-mail-in-lodge.md)
- Builds on: [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md), [ADR-0041](0041-maild-runs-as-a-lodge-app.md)
- Providers: [Microsoft redirect URI restrictions](https://learn.microsoft.com/en-us/entra/identity-platform/reply-url), [Google's device flow scopes](https://developers.google.com/identity/protocols/oauth2/limited-input-device#allowedscopes)
