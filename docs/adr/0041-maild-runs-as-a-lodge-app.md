# 0041 — maild runs as a lodge app

- **Status:** Proposed
- **Date:** 2026-10-18

## Context

maild is a per-user daemon on the user's desktop. The app and mailctl reach
it over a Unix socket that only its user may open, and every connection is
that user ([ADR-0002](0002-daemon-and-thin-clients.md),
[ADR-0013](0013-app-as-a-local-flatpak.md)). A Windows install runs its own
maild the same way ([ADR-0038](0038-windows-as-a-second-platform.md)).

On 2026-10-18 the user decided to run their first-party apps behind
**lodge** ([frostyard/lodge](https://github.com/frostyard/lodge);
[core ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)).
lodge is one tailnet-only web address for frostnotes, frostmail and a later
assistant:

- lodge is the only process on the network. Each app is a daemon on a Unix
  socket that only lodge's user may open, and runs as its own system user
  ([lodge ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md)).
- It signs browsers in by their tailnet identity, and passes the caller to
  the app in `Lodge-*` headers
  ([lodge ADR-0005](https://github.com/frostyard/lodge/blob/main/docs/adr/0005-browsers-sign-in-with-their-tailnet-identity.md)).
- It issues each app's MCP tokens and serves `/mcp/<id>`
  ([lodge ADR-0006](https://github.com/frostyard/lodge/blob/main/docs/adr/0006-agents-reach-each-app-through-its-own-mcp-endpoint.md)).
- It serves every app's web build from disk, and shows each app in a frame
  that stays loaded
  ([lodge ADR-0007](https://github.com/frostyard/lodge/blob/main/docs/adr/0007-lodge-serves-every-apps-web-build-from-disk.md),
  [ADR-0009](https://github.com/frostyard/lodge/blob/main/docs/adr/0009-apps-switch-in-frames-that-stay-loaded.md)).
- It serves untrusted bytes only from a second node, `lodge-content`,
  under a sandbox policy
  ([lodge ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md)).
  An app's frame may show `lodge-content` in its images, media and frames,
  and nothing else from another origin.

The [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md)
says what an app installs and serves. lodge's roadmap Phase 4 makes maild
its `mail` app, with a manifest that sets `content`, `mcp` and `key: "m"`.
The same day the user decided that **one maild on the server serves every
client, the desktop app included**. The friend's Windows install keeps a
local maild, so frostmail supports both.

Several things in frostmail assume the client and maild share a machine:

- The reader shows sanitized HTML in an `<iframe srcdoc>` that shares the
  app's origin and CSP ([ADR-0005](0005-html-mail-rendering.md)), with its
  images embedded as `data:` URLs
  ([ADR-0014](0014-images-embedded-in-the-reader-frame.md)). Its links are
  new-window requests the Tauri shell opens
  ([ADR-0036](0036-reader-links-open-as-new-window-requests.md)).
- Attachments reach the webview through `mailpart://`, which reads maild's
  parts cache from the same disk.
- `draft.attach` reads a file by its path, and `message.save` writes one,
  on maild's machine.
- Google sign-in redirects the browser to a loopback port on maild's
  machine ([ADR-0011](0011-sign-in-and-credentials.md)).
- Secrets go to the session's Secret Service, and notifications to the
  session bus ([design/desktop.md](../design/desktop.md#notifications)).

A system user on a server has no session bus, and a browser on another
machine reaches neither maild's disk nor its loopback.

## Decision

- **On a server, maild is lodge's `mail` app.**
  - `maild --lodge` serves HTTP/1.1 on the app socket the contract names
    for `mail`, through lodge's Go kit (`lodge/kit`).
  - It runs as the system user `frostmail`, with its data in
    `/var/lib/frostmail`.
  - It ships a manifest and a web build for lodge to install.

  Its manifest:

  ```json
  { "contract": 1, "id": "mail", "name": "Mail", "icon": "icon.svg",
    "version": "…", "user": "frostmail", "key": "m",
    "content": true, "contentLinks": true, "mcp": true }
  ```

  mail has no `device` routes. It never sets `remoteImages`, because only
  maild fetches remote images (ADR-0005).
- **maild listens on no network address.** The dev gateway
  (`internal/devgw`) stays on loopback, for development only.
- **The socket is what maild trusts.**
  - maild accepts a connection on the app socket only from lodge's user,
    through the kit's peer check.
  - It reads `Lodge-*` headers there only, and refuses a request without
    `Lodge-Caller`.
  - Each path takes one caller kind: `/api` session, `/content/` content
    and `/mcp` agent. Any other is 403.
- **maild gains caller kinds.**
  - `local` is the private socket, as today: mailctl, and the desktop app
    when it uses a maild on its own machine.
  - `session` is a connection on lodge's `/api`.
  - `agent` is an MCP session's connection inside maild.

  The IDL says which callers each method admits. A session may call every
  method the app uses, except the two that act on files of maild's
  machine: `draft.attach` and `message.save` are `local` only. A browser
  attaches by sending the file's bytes, and saves through a download from
  the content route.
- **The reader's page comes from `lodge-content`.**
  - `message.render` with `content` also keeps the reader's whole page
    for the content route, and answers its path. The page is the sanitized
    HTML inside the frame template the app uses today: the page's style and
    `<base target="_blank">`, with images embedded as `data:` URLs
    (ADR-0014).
  - The web app shows it in an `<iframe src>` on the content origin, in
    place of `srcdoc`.
  - `message.part` answers a content path for a decoded part, which the web
    app opens in a new tab or downloads.
  - Content paths are unguessable: a reader page's is a random token, and
    a part's carries an HMAC under a key maild draws at start.
- **ADR-0005's three layers hold in lodge:**
  - maild's sanitizer is unchanged.
  - The frame is another origin. lodge's content policy sandboxes it
    without scripts, and maild sends its own policy beside lodge's.
  - Both policies forbid every load but `data:` images and fonts.
- **Links open in a new tab.** The manifest sets `contentLinks` (lodge's
  decision D6), so lodge's content policy for mail is
  `sandbox allow-popups allow-popups-to-escape-sandbox`, never with
  scripts, same-origin, forms or top navigation. The sanitizer drops
  `base` and `target`, so every link targets a new tab, as ADR-0036's
  links do in the desktop app.
- **The reader's frame fills the pane.** A page on another origin cannot
  be measured, so in lodge the frame takes the reader's height below the
  message header and scrolls itself.
- **One maild for every client.**
  - The owner's maild runs on lodge's host. A browser and the phone's
    home-screen app reach it through lodge.
  - The desktop app becomes a lodge client too. Its bridge opens
    `wss://LODGE/app/mail/api` as a session, with the desktop's own node
    identity, and its Rust layer fetches attachments from `lodge-content`
    itself, so the webview still fetches nothing
    ([design/desktop.md](../design/desktop.md#on-a-maild-behind-lodge)).
  - A local maild stays the mode for an install outside the owner's
    tailnet, such as the friend's Windows install. The app uses one or the
    other, chosen in its settings.
  - On lodge's host, mailctl uses the private socket as `frostmail`
    (`sudo -u frostmail mailctl …`).
- **What stays mail's own:**
  - the JSON-RPC behind `/api`, whose frames lodge only moves;
  - the caller kinds, and which methods each admits;
  - the content routes and their paths;
  - the MCP tools behind `/mcp`, and their scope grammar. The engine
    enforces an agent's scope, so a tool's code alone never decides what an
    agent may reach. An agent sends mail only under a scope that says so;
  - sign-in, secrets, sync, rules, reminders and notifications.
- **On a server, secrets and notifications change place.**
  - With no Secret Service, maild keeps secrets in its file store
    (`/var/lib/frostmail/secrets.json`, 0600, owner `frostmail`).
  - With no session bus, maild sends no notifications itself. A client
    that receives its events shows them.
- **Google sign-in from another machine is open.** The loopback redirect
  of ADR-0011 cannot reach a server's maild from a browser elsewhere.
  [Plan 0011](../plans/0011-m8-mail-in-lodge.md) leaves it as an open
  question. Until it is answered, an account that signs in with a password
  can be added from lodge, and an OAuth account cannot.

## Consequences

- maild carries no tailnet node, TLS, sign-in, token store or file server.
  It depends on `lodge/kit`, which keeps `tailscale.com` out.
- Mail in a browser waits on lodge v1 and the kit (lodge's roadmap Phases
  1 and 2).
- **On the server:**
  - lodge's host runs maild, with the user's accounts and their secrets.
    A process running as `frostmail` can read every secret. A process
    running as lodge's user can claim any caller to maild, as lodge
    itself can.
  - maild's database and parts cache sit on the host's local disk, never
    on a network share (WAL needs shared memory).
- **The desktop app** keeps ADR-0005's rendering, ADR-0014's images and
  ADR-0036's links when it talks to a remote maild. Its Rust layer needs
  the network to reach lodge, and the Flatpak gains `--share=network`
  for it. The webview's CSP still forbids every fetch, so ADR-0005's third
  layer holds, but the Flatpak no longer stops the network on its own.
- **In lodge, the reader differs from the desktop's:**
  - the header stays put while the message scrolls in its own frame;
  - the switcher's chord, and the app's keys, do nothing while the focus
    is inside the message (lodge's shell.md records the limitation);
  - a link opens a browser tab, not the system browser.
- **The app's other windows become routes in the frame.** Compose,
  Settings, the event editor and the reminder window open today with
  `window.open`. From a frame, a new window would be a second copy of
  lodge's shell, so in lodge each is a route the frame shows in place.
- **Reader pages are kept in memory.** maild bounds them, and a page from
  before a restart is gone. The web app renders again when its page or
  its connection is new.
- The IDL gains method-level callers. That is an additive change to
  `tools/rpcgen`.
- `/mcp/mail` and its scope grammar are new work: frostmail had no MCP.
  The manifest sets `mcp` only once maild serves it.

## Alternatives considered

- **`srcdoc` in mail's frame, as in the desktop app:** untrusted bytes
  would run on lodge's own origin, which every app's frame shares. lodge's
  Rule 5 forbids it.
- **The reader's page from `/app/mail/x/`:** that is lodge's origin too.
- **A maild on every client:** the user decided on one maild, so that the
  phone, a browser and the desktop see one store, one outbox and one set
  of rules.
- **The desktop app showing lodge's page in its webview:** Tauri's IPC
  would be exposed to a remote origin, and the app would lose its own CSP
  and its `mailpart://` path.
- **Retiring the desktop app for lodge's home-screen app:** the user wants
  the desktop app, and the Windows install needs it anyway.
- **Measuring the frame with a script in the reader's page:** ADR-0005
  forbids scripts in the frame.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md), [plan 0011](../plans/0011-m8-mail-in-lodge.md), [design/overview.md](../design/overview.md), [design/rendering.md](../design/rendering.md), [design/desktop.md](../design/desktop.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md), [ADR-0005](0005-html-mail-rendering.md), [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0013](0013-app-as-a-local-flatpak.md), [ADR-0014](0014-images-embedded-in-the-reader-frame.md), [ADR-0036](0036-reader-links-open-as-new-window-requests.md), [ADR-0037](0037-fetch-what-the-reader-shows.md), [ADR-0038](0038-windows-as-a-second-platform.md); frostyard/core [ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)
- lodge: [ADR-0003](https://github.com/frostyard/lodge/blob/main/docs/adr/0003-lodge-is-the-only-process-on-the-network.md) to [ADR-0010](https://github.com/frostyard/lodge/blob/main/docs/adr/0010-untrusted-bytes-come-from-a-second-node.md), [the app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md), [roadmap Phase 4](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md#phase-4--mail-in-lodge)
- Model: frostnotes' [ADR-0026](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0026-notesd-runs-as-a-lodge-app.md)
