// Which layout the main window takes (docs/specs/ui.md, Narrow windows):
// one pane at a time below 600 CSS pixels of width, the phone in lodge's
// frame. app.css's `narrow:` variant uses the same query.
import { useSyncExternalStore } from "react";

/** NARROW is the media query of a narrow window. */
export const NARROW = "(max-width: 599px)";

function query(): MediaQueryList | null {
  return typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia(NARROW) : null;
}

/** isNarrow reports whether the window is narrow now. */
export function isNarrow(): boolean {
  return query()?.matches ?? false;
}

/** useNarrow reports whether the window is narrow, following its width. */
export function useNarrow(): boolean {
  return useSyncExternalStore((onChange) => {
    const q = query();
    q?.addEventListener("change", onChange);
    return () => q?.removeEventListener("change", onChange);
  }, isNarrow);
}

/** NarrowPane is the pane a narrow window shows in Mail. */
export type NarrowPane = "mailboxes" | "list" | "reader";

/** narrowPane is the pane for Mail's state: Mailboxes when asked for, the message when one is selected. */
export function narrowPane(mailboxesOpen: boolean, selected: readonly number[]): NarrowPane {
  if (mailboxesOpen) return "mailboxes";
  return selected.length === 1 ? "reader" : "list";
}
