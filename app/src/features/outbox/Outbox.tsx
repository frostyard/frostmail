// Sending feedback in the main window: the undo toast for a message waiting
// out its undo delay, and the outbox section for messages not yet sent
// (docs/specs/compose-ui.md, Undo toast and Outbox). Task T-0047 implements
// both; the stubs draw nothing.
import type { OutboxItem } from "../../rpc/gen/api";

/** UndoToastProps are the undo toast's inputs. */
export interface UndoToastProps {
  /** The message's subject; "(no subject)" shows when it is empty. */
  subject: string;
  /** Whole seconds until the message goes out; at 0 or less Undo is gone. */
  secondsLeft: number;
  onUndo: () => void;
}

/** UndoToast offers to cancel a message that is about to be sent. */
export function UndoToast(_props: UndoToastProps) {
  return null;
}

/** OutboxStatusProps are the outbox section's inputs. */
export interface OutboxStatusProps {
  items: OutboxItem[];
  /** The current time, for "Retrying in N min". */
  now: Date;
  onRetry: (id: number) => void;
  onEdit: (item: OutboxItem) => void;
}

/** OutboxStatus lists the messages that are not yet sent. */
export function OutboxStatus(_props: OutboxStatusProps) {
  return null;
}
