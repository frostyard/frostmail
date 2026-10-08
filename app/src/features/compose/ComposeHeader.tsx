// The compose window's header fields: To, Cc, Bcc, Subject and From
// (docs/specs/compose-ui.md, Layout: Header). Task T-0044 implements it;
// the stub draws nothing.
import type { Address, DraftContent, Identity } from "../../rpc/gen/api";

/** ComposeHeaderProps are the header's inputs. */
export interface ComposeHeaderProps {
  content: DraftContent;
  /** The identities of the draft's account; From shows when there are several. */
  identities: Identity[];
  showBcc: boolean;
  /** Called with the fields that changed. */
  onChange: (patch: Partial<DraftContent>) => void;
  /** Recipient suggestions, passed to each recipient field. */
  suggest: (prefix: string) => Promise<Address[]>;
  /** Focus the To field when the window opens. */
  autoFocusTo?: boolean;
}

/** ComposeHeader is the stack of header rows above the editor. */
export function ComposeHeader(_props: ComposeHeaderProps) {
  return null;
}
