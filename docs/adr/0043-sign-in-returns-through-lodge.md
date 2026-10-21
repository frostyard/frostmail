# 0043 — OAuth sign-in returns through lodge, or by a pasted address

- **Status:** Proposed
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
  - The "Mobile and desktop applications" platform takes custom redirect
    URIs. Entra treats a redirect under it as a public client's, which
    redeems a code with PKCE and no secret (not yet tried with an `https`
    URI: the first sign-in confirms it). Frostmail's client is a public
    client, and the owner registered it.
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
- **lodge** ([app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md)):
  - `https://LODGE/<id>/<route>` as a top-level page is the shell, which
    opens the app's frame at `<route>` with its query and fragment. The
    route is the app's to define, except the first segments `api` and
    `x`.
  - lodge passes a request's query to the app unchanged and reads nothing
    from it.
  - The browser reaches it with its tailnet identity, on any tailnet
    device, and the frame calls maild over `/api` as a `session` caller.
  - lodge gives an app no route that an outside server can call, and maild
    listens on no network address (ADR-0041).

## Decision

- **A provider's redirect is loopback or lodge's.** maild keeps a redirect
  for each provider:
  - **loopback** (the default) is ADR-0011's: maild listens on its
    loopback for the one sign-in, as today.
  - **lodge** is an `https` URL at mail's route `/signin` on lodge, such
    as `https://lodge.example.ts.net/mail/signin`. maild listens on no
    port for it.
  - The owner sets it with `oauth.setRedirect {provider, url}` (and
    `mailctl oauth set-redirect`), after the URL is registered in the
    provider's client. An empty URL is loopback. maild accepts only an
    `https` URL whose path is `/mail/signin`.
- **The sign-in finishes in maild, from any client.** A new method,
  `account.finishSignIn {url}`, takes the address the provider sent the
  browser to. It admits `local` and `session` callers, never `agent`.
  - maild reads `state`, `code` and `error` from the address's query. It
    finds the waiting sign-in by `state`, compared in constant time, and
    finishes it as the loopback listener does: it exchanges the code with
    the same redirect URI and the PKCE verifier, stores the refresh token,
    clears "Sign in again", and reconnects the account.
  - A sign-in finishes once, by whichever way comes first: the loopback
    listener or `account.finishSignIn`. An unknown or finished `state` is
    `invalidParams`, and maild logs no code.
  - `account.authorize`'s result gains `redirect`: `loopback` or `lodge`.
- **With lodge's redirect, nothing is manual.** The web app opens the
  authorization URL in a new tab. The provider sends that tab to
  `https://LODGE/mail/signin?code=…&state=…`. lodge's shell opens mail at
  the route `/signin`, which calls `account.finishSignIn` with its query,
  and at once reports the route `/settings` with `replace`, so the code
  leaves the address bar and the history. The tab then says the account is
  signed in, and the first tab sees `account.changed`.
- **With loopback, a client off maild's machine offers the paste.** When
  `redirect` is `loopback` and the client is not on maild's machine (the
  web build, or the desktop app on a remote maild), Settings shows
  "After you sign in, your browser cannot open the last page. Copy its
  address and paste it here", with a field that calls
  `account.finishSignIn`. It works for every provider with the clients
  registered today. The desktop app on its own maild keeps today's flow,
  with no field.
- **The owner's setup:**
  - Microsoft: add `https://lodge.goat-snake.ts.net/mail/signin` to
    Frostmail's Entra app under "Mobile and desktop applications", then
    set it as Microsoft's redirect. The built-in client ID stays, so the
    existing grant stays.
  - Google: the two Gmail accounts stay on loopback and the paste. Lodge's
    redirect needs a "Web application" client with the URL registered, and
    every Google account signs in again with the new client. The owner
    decides when.

## Consequences

- An Outlook sign-in from lodge needs no manual step on any tailnet
  device, the phone included. A Gmail sign-in needs one paste until the
  owner registers a Web client.
- Phase 7's sign-in question is answered.
- The code passes through the browser's address bar and lodge's shell
  page. It is single-use, short-lived and useless without the PKCE
  verifier, which never leaves maild. The route is replaced at once.
- A lodge URL is one registration per lodge host in each provider's client.
  Frostmail's Microsoft client can hold 100: enough for the owner and a few
  friends, not for a public release. M6's verified clients revisit this.
- The paste is a manual step and is clumsy on a phone, but it always
  works, and it is also the way out when a registered URL is wrong.
- maild's sign-in flows are found by `state`, not only by account. The
  loopback listener stays for the desktop app on its own maild and for
  Windows (ADR-0038).
- `account.finishSignIn`, `oauth.setRedirect` and `AuthorizeResult.redirect`
  are additive in protocol 1.

## Alternatives considered

- **The device-code flow:** Google does not allow Gmail's scope in it, so
  it would serve Microsoft only, and tenants often block it. The user also
  types a code, which is a manual step too.
- **A relay page at one public URL,** which sends the browser on to the
  lodge host named in `state` (Microsoft's documented answer to the
  redirect limit): one registration serves every host, but the code would
  pass through a page outside the tailnet, and the page could become an
  open redirector. Revisit with M6's verified clients.
- **A lodge route that the provider's server calls:** the provider never
  calls the redirect itself; the browser does. lodge's shell route is
  enough, and the contract needs no change.
- **Sign in on the desktop app and hand the grant to the server:** it needs
  the desktop app, a second maild and a way to move a refresh token
  between machines. lodge exists so that a browser is enough.
- **A tunnel to maild's loopback for each sign-in:** a manual step on every
  device, and impossible from a phone.

## References

- Shapes: [design/accounts.md](../design/accounts.md#sign-in), [specs/lodge-app.md](../specs/lodge-app.md), [specs/rpc-api.md](../specs/rpc-api.md), [plan 0011](../plans/0011-m8-mail-in-lodge.md)
- Builds on: [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md), [ADR-0041](0041-maild-runs-as-a-lodge-app.md)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [shell](https://github.com/frostyard/lodge/blob/main/docs/specs/shell.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Providers: [Microsoft redirect URI restrictions](https://learn.microsoft.com/en-us/entra/identity-platform/reply-url), [Google's device flow scopes](https://developers.google.com/identity/protocols/oauth2/limited-input-device#allowedscopes)
