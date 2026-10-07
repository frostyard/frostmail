// The reader's message header, attachment strip and remote-content banner
// (docs/specs/ui.md, Reader). Task T-0031 implements all three; the stubs
// show the bare minimum.
import type { Message, Part } from "../../rpc/gen/api";

/** MessageHeaderProps are a header's inputs. */
export interface MessageHeaderProps {
  message: Message;
}

/** MessageHeader shows the sender, subject, recipients and date. */
export function MessageHeader(props: MessageHeaderProps) {
  return <header>{props.message.summary.subject}</header>;
}

/** AttachmentStripProps are the attachment strip's inputs. */
export interface AttachmentStripProps {
  parts: Part[];
  onOpen: (part: Part) => void;
}

/** AttachmentStrip shows a message's attachments as chips. */
export function AttachmentStrip(_props: AttachmentStripProps) {
  return null;
}

/** RemoteBannerProps are the remote-content banner's inputs. */
export interface RemoteBannerProps {
  /** Remote resources not loaded. */
  remote: number;
  /** Tracking images removed. */
  trackers: number;
  /** A load is in progress. */
  loading: boolean;
  onLoad: () => void;
}

/** RemoteBanner offers to load remote content. */
export function RemoteBanner(_props: RemoteBannerProps) {
  return null;
}
