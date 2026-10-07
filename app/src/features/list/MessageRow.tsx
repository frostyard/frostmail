// One row of the message list (docs/specs/ui.md, Message list). Task T-0029
// implements MessageRow; the stub shows the subject only.
import type { MessageSummary } from "../../rpc/gen/api";

/** ROW_HEIGHT is the fixed height of a list row in pixels. */
export const ROW_HEIGHT = 84;

/** SelectMode says how a click changes the selection. */
export type SelectMode = "replace" | "toggle" | "range";

/** MessageRowProps are a row's inputs. */
export interface MessageRowProps {
  message: MessageSummary;
  selected: boolean;
  /** The list has keyboard focus: selected rows use the accent color. */
  focused: boolean;
  /** Conversation mode: show the thread count badge when above 1. */
  showThreadCount: boolean;
  /** The clock dates are formatted against. */
  now: Date;
  onSelect: (id: number, mode: SelectMode) => void;
  onContextMenu: (id: number, x: number, y: number) => void;
}

/** MessageRow renders one message summary. */
export function MessageRow(props: MessageRowProps) {
  return (
    <div role="option" tabIndex={-1} aria-selected={props.selected} style={{ height: ROW_HEIGHT }}>
      {props.message.subject}
    </div>
  );
}
