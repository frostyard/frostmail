// The compose window's header fields: To, Cc, Bcc, Subject and From
// (docs/specs/compose-ui.md, Layout: Header). Bcc shows only while it is
// shown and From only when the account has several identities. The header is
// controlled: it renders from `content` and reports each edit through
// `onChange`; the compose container owns the draft.
import type { ReactNode } from "react";

import type { Address, DraftContent, Identity } from "../../rpc/gen/api";
import { RecipientField } from "./RecipientField";

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

/** identityText is an identity's option label: "Name <email>", or the email. */
function identityText(identity: Identity): string {
  return identity.name === "" ? identity.email : `${identity.name} <${identity.email}>`;
}

/** Row is one header row: the label column and the field beside it. */
function Row(props: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-h-[30px] items-center border-b border-separator px-4">
      <span
        data-header-label
        aria-hidden="true"
        className="w-16 shrink-0 pr-2 text-right text-[13px] leading-[18px] text-secondary"
      >
        {props.label}:
      </span>
      {props.children}
    </div>
  );
}

/** ComposeHeader is the stack of header rows above the editor. */
export function ComposeHeader(props: ComposeHeaderProps) {
  const { content, identities, showBcc, onChange, suggest, autoFocusTo } = props;
  return (
    <div>
      <Row label="To">
        <RecipientField
          label="To"
          value={content.to}
          suggest={suggest}
          autoFocus={autoFocusTo}
          onChange={(to) => onChange({ to })}
        />
      </Row>
      <Row label="Cc">
        <RecipientField label="Cc" value={content.cc} suggest={suggest} onChange={(cc) => onChange({ cc })} />
      </Row>
      {showBcc && (
        <Row label="Bcc">
          <RecipientField label="Bcc" value={content.bcc} suggest={suggest} onChange={(bcc) => onChange({ bcc })} />
        </Row>
      )}
      <Row label="Subject">
        <input
          type="text"
          aria-label="Subject"
          value={content.subject}
          className="min-w-0 flex-1 border-none bg-transparent text-[13px] leading-[18px] text-primary outline-none"
          onChange={(event) => onChange({ subject: event.target.value })}
        />
      </Row>
      {identities.length > 1 && (
        <Row label="From">
          <select
            aria-label="From"
            value={String(content.identityId)}
            className="min-w-0 border-none bg-transparent text-[13px] leading-[18px] text-primary outline-none"
            onChange={(event) => onChange({ identityId: Number(event.target.value) })}
          >
            {identities.map((identity) => (
              <option key={identity.id} value={String(identity.id)}>
                {identityText(identity)}
              </option>
            ))}
          </select>
        </Row>
      )}
    </div>
  );
}
