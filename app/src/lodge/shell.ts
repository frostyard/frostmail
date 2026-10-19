// Mail's half of lodge's shell protocol (docs/specs/lodge-app.md, The web
// build): the route follows the sidebar's source and the selection, or the
// window the frame shows (frameWindow.ts); a navigate message moves them,
// and the theme follows the shell's.
import { isNarrow } from "../app/narrow";
import { type Source, sourceKey, sourceTitle, useMail, useUI } from "../data/stores";
import { type FrameWindow, useFrameWindow, windowFromRoute, windowRoute } from "./frameWindow";
import type { Lodge } from "./lodge";
import { parseRoute, routeOf } from "./routes";

/** applyTheme sets the page's scheme, which tokens.css reads from data-theme. */
export function applyTheme(scheme: "light" | "dark", root: HTMLElement = document.documentElement): void {
  root.dataset.theme = scheme;
}

/** go shows a route: a window, or a source and the message it names. */
export function go(route: string): void {
  const w = windowFromRoute(route.split("#", 1)[0] ?? "");
  if (w) {
    useFrameWindow.setState({ open: w });
    return;
  }
  useFrameWindow.setState({ open: null });
  const where = parseRoute(route);
  const ui = useUI.getState();
  if (sourceKey(ui.source) !== sourceKey(where.source)) ui.setSource(where.source);
  if (useUI.getState().mailboxesOpen) useUI.getState().openMailboxes(false);
  const selected = where.message === undefined ? [] : [where.message];
  if (useUI.getState().selected.join(",") !== selected.join(","))
    useUI.getState().select(selected, where.message ?? null);
}

const WINDOW_TITLES: Record<FrameWindow["kind"], string> = {
  compose: "New Message",
  settings: "Settings",
  event: "Event",
  reminders: "Reminders",
};

/** Place is where the app is: its route, title, and what a new route there replaces. */
interface Place {
  route: string;
  title: string;
  /** Routes of one place replace each other in history; a new place pushes. */
  key: string;
  /**
   * 1 for a message a narrow window shows in place of its list, else 0:
   * going deeper within a place pushes too, so Back returns to the list
   * (docs/specs/ui.md, Narrow windows).
   */
  depth: number;
}

function place(source: Source, selected: readonly number[], w: FrameWindow | null): Place {
  if (w) {
    const route = windowRoute(w);
    return { route, title: WINDOW_TITLES[w.kind], key: `window:${route}`, depth: 0 };
  }
  return {
    route: routeOf(source, selected),
    title: sourceTitle(source, useMail.getState()),
    key: sourceKey(source),
    depth: selected.length === 1 && isNarrow() ? 1 : 0,
  };
}

function current(): Place {
  const ui = useUI.getState();
  return place(ui.source, ui.selected, useFrameWindow.getState().open);
}

// Whether the history entry under the shown message is its list's, which
// a narrow window's report pushed: then Back is history's own.
let overList = false;

/**
 * backToList takes a narrow window from a message back to its list by
 * history, when the entry under it is the list's, so the shell's Back and
 * the toolbar's agree. It reports whether it went.
 */
export function backToList(): boolean {
  if (!overList) return false;
  overList = false;
  window.history.back();
  return true;
}

/**
 * wireShell opens the app at route (the frame's own path under
 * /app/mail), then keeps the shell's route, history and theme in step with
 * the app. It returns the unsubscribe.
 */
export function wireShell(l: Lodge, route: string): () => void {
  applyTheme(l.theme);
  go(route);
  let last = current();
  l.route(last.route, last.title, { replace: true });
  overList = false;
  let navigating = false;
  const report = () => {
    if (navigating) return;
    const next = current();
    if (next.route === last.route) return;
    const same = next.key === last.key;
    const deeper = same && next.depth > last.depth;
    l.route(next.route, next.title, { replace: same && !deeper });
    // Moving from message to message keeps the list under them.
    overList = deeper || (overList && same && next.depth === 1);
    last = next;
  };
  const offUI = useUI.subscribe(report);
  const offWindow = useFrameWindow.subscribe(report);
  const offNavigate = l.on("navigate", ({ route: r }) => {
    // The shell already shows this route: moving there reports nothing.
    navigating = true;
    try {
      go(r);
    } finally {
      navigating = false;
    }
    last = current();
    overList = false;
  });
  const offTheme = l.on("theme", ({ scheme }) => applyTheme(scheme));
  return () => {
    offUI();
    offWindow();
    offNavigate();
    offTheme();
  };
}
