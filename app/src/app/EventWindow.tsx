// The event editor window (docs/specs/pim-ui.md, Event editor window): an
// event, one occurrence of a series, or a new event, in an editor of its
// own. The editor comes with task card T-0148.
import type { EditorTarget } from "./eventWindow";

/** EventWindow edits its target's event. */
export function EventWindow({ target }: { target: EditorTarget }) {
  return (
    <div data-tauri-drag-region className="flex h-full items-center justify-center bg-window text-empty text-secondary">
      {target.kind === "new" ? "New Event" : "Event"}: the editor comes with task card T-0148.
    </div>
  );
}
