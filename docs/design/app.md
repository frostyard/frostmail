# The app in Lodge

Living document. Rationale: [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
[ADR-0063](../adr/0063-make-the-nsgo-client-the-only-client.md),
[ADR-0005](../adr/0005-html-mail-rendering.md).
Contracts: [specs/lodge-app.md](../specs/lodge-app.md#the-web-build),
[specs/ui.md](../specs/ui.md), [specs/rpc-api.md](../specs/rpc-api.md).

## Overview

FrostMail has one client, the NSGo client (`cmd/frostmail-nsgo`). On Linux
and Windows it is the desktop app ([design/desktop.md](desktop.md)); in
Lodge the same Go code runs in the browser, compiled to WebAssembly by
NSGo's own tool, as Lodge's `mail` app. It replaced the React app in Lodge
in [plan 0017](../plans/0017-nsgo-as-the-only-client.md)'s Phase 3.5, which
lists what the React app did that this build does not.

The web build reaches maild only through Lodge, and holds no mail of its
own: lists are views kept by maild, messages are fetched when shown, and
every change goes back to maild as an API call, as on the desktop.

```
lodge's shell ── frame /app/mail/ ── index.html, src/nsgo-lodge.ts (the loader)
                                         │ window.frostmailLodge (the bridge)
                                         ▼
                         the NSGo client (assets/app.*.wasm)
                                         │ internal/desktop/conn
                                         ▼
                 /app/mail/api (WebSocket, a session) ──► maild --lodge
```

## The build

`make web-build` writes `build/web/`, which `make deb` installs as
`/usr/share/lodge/apps/mail/web/`:

1. `go tool nsgo build -target web` builds the client into
   `build/nsgo-web/FrostMail-<version>-web/` (`nsgo.yaml`'s version): its
   Wasm, NSGo's loader script `nsgo.*.js`, Go's `wasm_exec.*.js`, the
   third-party notices, and `nsgo.json`, which names them.
2. Vite builds `app/` for the base `/app/mail/` in the nsl machine (CI runs
   it on the runner, `IN_NSL='sh -c'`): `index.html` and the loader
   `assets/index-*.js`.
3. `nsgo.json` goes beside `index.html`, NSGo's files into `assets/`, and
   `packaging/icons/icon.svg` beside them.

NSGo's own `index.html`, `mount.*.js`, icons and web manifest are not
copied: the loader takes their place. `make deb-check` holds the layout:
the page, `nsgo.json`, the Wasm and NSGo's scripts are there, and no
stylesheet, font or file holding React's names is
(`packaging/deb-check.py`'s `--glob`, `--absent` and `--no-text`).

`app/` holds only the loader: `index.html` (the page's print styles), and
`src/nsgo-lodge.ts` with `src/permalink.ts`, on `@frostyard/lodge-client`.
`app/notify` is the Windows toast notifier (Rust), and builds apart.

## The loader and its bridge

`src/nsgo-lodge.ts` connects to the shell with `connectLodge({app:
"mail"})`, sets `window.frostmailLodge`, fetches `nsgo.json` and mounts the
client with NSGo's loader, in the shell's theme, which it follows after.
The client calls the bridge from `platform_web.go`:

| Call | Does |
| --- | --- |
| `onRoute(callback)` | returns the frame's route and sends later `navigate` routes to `callback` |
| `report(route, title, replace)` | the shell's `route` |
| `context(id, messageId, title)` | the shell's `context`: the message's permanent route, or none |
| `backToList()` | `history.back()`, for a narrow window's Back from a message |
| `printMessage(subject, headers, text)` | prints an article of those 3 alone, with the page's print styles |
| `contentURL(path)` | the shell's `contentURL` |
| `openPart(path)`, `download(path)` | `lodge.open` of the content URL, the second with `?download=1` |

## The client in the browser

- **Connection.** `platformTarget` is Lodge at the page's origin;
  `internal/desktop/conn` opens `/app/mail/api` there (`socket_web.go`,
  which refuses any other origin), says `rpc.hello` as `FrostMail NSGo`,
  pings every 30 s and reconnects every second, as on the desktop.
- **Routes** (`routes.go`): the source and the open message, the modules
  (`/calendar`, `/people`, `/tasks`, `/activity`, `/settings`), and the
  panels with a route of their own (`/compose/<id>`, `/event/…`,
  `/reminders`). `platform_web.go` reports them, replacing within a place
  and pushing a new one, and pushing a message a narrow window (under 800
  wide) shows in place of its list, which Back undoes by history.
- **The reader** shows the page maild keeps for the message, from
  `contentURL(contentPath)`, in NSGo's web view with JavaScript off
  (`bodyView`); a message with no page shows its text.
- **Attachments and Save As** open or download content URLs through the
  bridge. There are no thumbnails, no server panel and no tray.
- **Notifications** of `message.announced` go through NSGo's notification
  call, which in a browser is the Notification API; Settings →
  Notifications asks for the permission.
- **Storage** is NSGo's: local storage under `nsgo:org.frostyard.Frostmail:`.

## Dev and test

The app runs only in lodge, so it is developed in lodge's development build
([lodge's specs/lodge-http.md](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md),
The development build), with the sibling checkout `../lodge`:

1. In `../lodge`: `make e2e-build`, `make shell-build` (nsl), then
   `FAKE_APPS=0 dev/e2e.sh ROOT`. Keep ROOT short: a socket's path is at
   most 108 bytes.
2. In this repository: `make web-build` builds the app for lodge's
   `/app/mail/`, and `dev/lodge/run.sh ROOT` stages it with maild, then
   runs `maild --lodge` on generated mail under ROOT.
3. `LODGE_ROOT=ROOT ../lodge/build/lodgectl reload` picks the app up; open
   `http://127.0.0.1:8480/mail`. After a change, stop `dev/lodge/run.sh`,
   run `make web-build` and start it again: it stages the new build, and
   lodge serves it at the next page load.

`tools/lodgeprobe -socket ROOT/run/lodge/apps/mail.sock` checks that
maild as lodge's app answers: health, a session, and a reader page from
the content route ([specs/lodge-app.md](../specs/lodge-app.md)).
`dev/lodge/run.sh` shows the environment `maild --lodge` runs with under a
lodge root. Run it with `FROSTMAIL_SECRETS=file`, or its test passwords
move into the desktop's keyring (docs/design/accounts.md, Secrets);
`dev/incus/mailtest.sh demo USER` delivers realistic demo mail.

- `dev/lodge/nsgo-smoke.py TOOLS --deb DEB` runs a disposable Lodge on
  what the `.deb` installs, maild and the web build, and drives it in
  headless Firefox: the inbox, the reader's frame, Settings, Back, a
  reload and a draft's route. Without `--deb` it runs on `make
  nsgo-lodge-app`'s staged directory and a maild built from the tree.
- `make nsgo-test` builds the client for `js/wasm` too.
- `make ui-check` runs Biome, the TypeScript check and Vitest on the
  loader, in the nsl machine.

## Operational notes

- A browser keeps the appearance, the window's panes and how the list is
  shown in its local storage; clearing it resets them.
- If maild is not running, the client shows the connection's state and
  keeps retrying; a maild started later is picked up without a reload.
