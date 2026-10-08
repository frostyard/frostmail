// The attachment strip under the compose window's editor
// (docs/specs/compose-ui.md, Layout: Attachments). Task T-0046 implements
// it; the stub draws nothing.
import type { DraftAttachment } from "../../rpc/gen/api";

/** PendingAttachment is a file still being attached. */
export interface PendingAttachment {
  /** Unique among the pending files; the React key. */
  key: string;
  filename: string;
}

/** ComposeAttachmentsProps are the attachment strip's inputs. */
export interface ComposeAttachmentsProps {
  attachments: DraftAttachment[];
  pending: PendingAttachment[];
  /** The size limit in bytes; the total turns red above it. */
  limit: number;
  onRemove: (id: number) => void;
}

/** ComposeAttachments lists a draft's attachments. */
export function ComposeAttachments(_props: ComposeAttachmentsProps) {
  return null;
}
