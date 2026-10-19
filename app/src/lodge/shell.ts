// Mail's half of lodge's shell protocol (docs/specs/lodge-app.md, The web
// build): the route follows the sidebar's source and the selection, or the
// window the frame shows (frameWindow.ts); a navigate message moves them,
// and the theme follows the shell's.
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
}

function place(source: Source, selected: readonly number[], w: FrameWindow | null): Place {
  if (w) {
    const route = windowRoute(w);
    return { route, title: WINDOW_TITLES[w.kind], key: `window:${route}` };
  }
  return { route: routeOf(source, selected), title: sourceTitle(source, useMail.getState()), key: sourceKey(source) };
}

function current(): Place {
  const ui = useUI.getState();
  return place(ui.source, ui.selected, useFrameWindow.getState().open);
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
  let navigating = false;
  const report = () => {
    if (navigating) return;
    const next = current();
    if (next.route === last.route) return;
    l.route(next.route, next.title, { replace: next.key === last.key });
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
  });
  const offTheme = l.on("theme", ({ scheme }) => applyTheme(scheme));
  return () => {
    offUI();
    offWindow();
    offNavigate();
    offTheme();
  };
}
