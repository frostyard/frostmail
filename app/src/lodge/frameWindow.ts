// The app's other windows inside lodge (docs/specs/lodge-app.md, The web
// build; plan 0011's Phase 4). From lodge's frame a new window would be a
// second copy of the shell, so Compose, Settings, the event editor and the
// reminder window are routes the frame shows in place of the main window,
// which stays mounted, hidden, underneath.
import { create } from "zustand";

import { type EditorTarget, eventFromRoute, eventRoute } from "../app/openEvent";

/** FrameWindow is a window the frame shows in place of the main window. */
export type FrameWindow =
  | { kind: "compose"; draftId: number; fresh: boolean }
  | { kind: "settings" }
  | { kind: "event"; target: EditorTarget }
  | { kind: "reminders" };

/** useFrameWindow holds the window shown, or null for the main window. */
export const useFrameWindow = create<{ open: FrameWindow | null }>(() => ({ open: null }));

/** frameWindowOpen reports whether a window covers the main window. */
export function frameWindowOpen(): boolean {
  return useFrameWindow.getState().open !== null;
}

/** openFrameWindow shows w in the frame. */
export function openFrameWindow(w: FrameWindow): void {
  useFrameWindow.setState({ open: w });
}

/** closeFrameWindow goes back to the main window. */
export function closeFrameWindow(): void {
  useFrameWindow.setState({ open: null });
}

/** windowRoute is a window's route. */
export function windowRoute(w: FrameWindow): string {
  switch (w.kind) {
    case "compose":
      return `/compose/${w.draftId}${w.fresh ? "?fresh" : ""}`;
    case "settings":
      return "/settings";
    case "event":
      return eventRoute(w.target);
    case "reminders":
      return "/reminders";
  }
}

/** windowFromRoute is the window a route names, or null for the main window's routes. */
export function windowFromRoute(route: string): FrameWindow | null {
  const compose = /^\/compose\/([1-9][0-9]{0,15})(\?fresh)?$/.exec(route);
  if (compose?.[1]) return { kind: "compose", draftId: Number(compose[1]), fresh: compose[2] !== undefined };
  if (route === "/settings") return { kind: "settings" };
  if (route === "/reminders") return { kind: "reminders" };
  if (route.startsWith("/event/")) {
    const target = eventFromRoute(route);
    if (target && (target.kind === "new" || Number.isSafeInteger(target.id))) return { kind: "event", target };
  }
  return null;
}
