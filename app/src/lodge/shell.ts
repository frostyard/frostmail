// Mail's half of lodge's shell protocol (docs/specs/lodge-app.md, The web
// build): the route follows the sidebar's source and the selection, a
// navigate message moves them, and the theme follows the shell's.
import { type Source, sourceKey, sourceTitle, useMail, useUI } from "../data/stores";
import type { Lodge } from "./lodge";
import { parseRoute, routeOf } from "./routes";

/** applyTheme sets the page's scheme, which tokens.css reads from data-theme. */
export function applyTheme(scheme: "light" | "dark", root: HTMLElement = document.documentElement): void {
  root.dataset.theme = scheme;
}

/** go shows a route: its source, and the message it names. */
export function go(route: string): void {
  const where = parseRoute(route);
  const ui = useUI.getState();
  if (sourceKey(ui.source) !== sourceKey(where.source)) ui.setSource(where.source);
  const selected = where.message === undefined ? [] : [where.message];
  if (useUI.getState().selected.join(",") !== selected.join(","))
    useUI.getState().select(selected, where.message ?? null);
}

function title(source: Source): string {
  return sourceTitle(source, useMail.getState());
}

/**
 * wireShell opens the app at route (the frame's own path under
 * /app/mail), then keeps the shell's route, history and theme in step with
 * the app. It returns the unsubscribe.
 */
export function wireShell(l: Lodge, route: string): () => void {
  applyTheme(l.theme);
  go(route);
  let last = routeOf(useUI.getState().source, useUI.getState().selected);
  let lastSource = sourceKey(useUI.getState().source);
  l.route(last, title(useUI.getState().source), { replace: true });
  let navigating = false;
  const offUI = useUI.subscribe((s) => {
    if (navigating) return;
    const next = routeOf(s.source, s.selected);
    if (next === last) return;
    // A new source is a history entry; a new message in it replaces it.
    const key = sourceKey(s.source);
    l.route(next, title(s.source), { replace: key === lastSource });
    last = next;
    lastSource = key;
  });
  const offNavigate = l.on("navigate", ({ route: r }) => {
    // The shell already shows this route: moving there reports nothing.
    navigating = true;
    try {
      go(r);
    } finally {
      navigating = false;
    }
    const ui = useUI.getState();
    last = routeOf(ui.source, ui.selected);
    lastSource = sourceKey(ui.source);
  });
  const offTheme = l.on("theme", ({ scheme }) => applyTheme(scheme));
  return () => {
    offUI();
    offNavigate();
    offTheme();
  };
}
