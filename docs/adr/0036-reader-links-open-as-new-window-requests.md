# 0036 — Links in the reader's frame open as new-window requests

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

[ADR-0005](0005-html-mail-rendering.md) shows a message in an
`<iframe srcdoc sandbox="allow-same-origin">`, never with `allow-scripts`,
and grants same-origin so the parent can size the frame and intercept
clicks. `MessageFrame` added capturing click listeners to the frame's
document that called the `open_link` command.

On 2026-10-14 the user reported that clicking a link in a message opened
nothing in the installed app. An end-to-end probe in the built app on
WebKitGTK 2.54 (nsl) found the cause. WebKitWebDriver clicked a link inside
the frame, and neither a document nor a window listener added by the
parent ran, while the parent's `ResizeObserver` on the same document did.
WebKit runs no event listener on a document whose frame is sandboxed
without `allow-scripts`, whoever added it. The click's own navigation
reached `on_navigation`, which refuses everything but the app, so nothing
happened. Vitest runs in happy-dom, which ignores the sandbox, and
browsers on the dev gateway do run the listeners, so neither showed the
fault.

Tauri always installs its own handler for WebKit's `decide-policy`
signal and ends the signal there. `on_navigation` receives only the URL.
It cannot tell a click from a navigation the message starts by itself,
such as a nested frame's load or a `meta refresh` that got past the
sanitizer.

## Decision

- The frame's sandbox is `allow-same-origin allow-popups`, and its
  template sets `<base target="_blank">`. maild's sanitizer drops `base`
  and `target` from messages, so every link in a message targets a new
  window.
- A click on a link asks the webview for a new window. The main window's
  `on_new_window` handler (`commands::open_new_window`) opens http, https
  and mailto URLs with the opener plugin, the same path as `open_link`.
  It logs a URL it refuses or fails to open, and it always answers
  `Deny`: no window is created and the frame stays as it was.
- The frame's click listeners stay. Browsers run them, and there they
  cancel the navigation and call `open_link` as before.
- The page logs `open_link` errors with `console.warn` instead of
  dropping them.

## Consequences

- Links open in the system browser from the built app and the Flatpak,
  where xdg-open calls the OpenURI portal
  ([ADR-0013](0013-app-as-a-local-flatpak.md)).
- `allow-popups` lets the frame ask for a window, but without scripts only
  the user's click on a link can: the sanitizer drops forms, and the CSP
  has `form-action 'none'`. Every request is denied. Without
  `allow-popups-to-escape-sandbox`, a popup would inherit the sandbox
  anyway.
- `app/e2e/links.e2e.ts` clicks links in the frame with stand-in
  launchers on the app's PATH. The hostile suite checks that viewing
  hostile mail opens nothing.
- The shell, not the page, decides what a frame link does. An in-app
  action for a link, such as composing a `mailto:` in Frostmail, belongs
  in `open_new_window`, or in an event it sends to the page.
- Only the main window handles new-window requests, because no other
  window shows message HTML. A window that comes to show it needs the
  same handler.

## Alternatives considered

- **`allow-scripts`:** message scripts would run next to the app's page.
  ADR-0005 forbids it.
- **Opening refused navigations from `on_navigation`:** the handler sees
  only a URL, so a navigation the message starts by itself would open the
  browser without a click.
- **A `decide-policy` handler of the app's own that reads the navigation
  type and user gesture:** Tauri's handler runs first and ends the signal.
  Going around it means reaching into wry's signal handlers.
- **`allow-top-navigation-by-user-activation` with
  `<base target="_top">`:** WebKit would require a click, but the
  frame's own navigations still reach `on_navigation` as bare URLs.
- **Catching clicks in the parent over a frame with
  `pointer-events: none`:** text selection and the context menu inside
  messages would stop working.

## References

- Shapes: [design/app.md](../design/app.md) (Reader)
- Builds on: [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0013](0013-app-as-a-local-flatpak.md)
