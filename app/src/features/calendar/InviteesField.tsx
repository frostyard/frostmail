import { Check, CircleHelp, X } from "lucide-react";
import { isValidAddress } from "../../lib/addressParse";
import type { DraftInvitee } from "../../lib/eventDraft";
import type { Address } from "../../rpc/gen/api";
import { RecipientField } from "../compose/RecipientField";

/** InviteesFieldProps supplies invitees and the chosen calendar's invitation capability. */
export interface InviteesFieldProps {
  invitees: DraftInvitee[];
  onChange: (next: DraftInvitee[]) => void;
  suggest: (prefix: string) => Promise<Address[]>;
  available: boolean;
  disabled?: boolean;
}

function InviteeRow({
  invitee,
  disabled,
  onToggle,
  onRemove,
}: {
  invitee: DraftInvitee;
  disabled: boolean;
  onToggle: () => void;
  onRemove: () => void;
}) {
  const { email, name, role, answer } = invitee;
  const invalid = !isValidAddress(email);
  const title = invalid ? `${email} is not a valid address` : name ? `${name} <${email}>` : email;
  return (
    <li className="flex h-7 items-center gap-2 text-[13px] leading-[18px]">
      <span title={title} className={`min-w-0 flex-1 truncate ${invalid ? "text-flag-1" : "text-primary"}`}>
        {name || email}
      </span>
      {answer === "accepted" && <Check size={14} aria-label="Accepted" className="shrink-0 text-flag-4" />}
      {answer === "declined" && <X size={14} aria-label="Declined" className="shrink-0 text-flag-1" />}
      {answer === "tentative" && <CircleHelp size={14} aria-label="Maybe" className="shrink-0 text-flag-2" />}
      <button
        type="button"
        aria-pressed={role === "optional"}
        disabled={disabled}
        className="text-[12px] leading-4 text-secondary aria-pressed:text-accent disabled:opacity-40"
        onClick={onToggle}
      >
        Optional
      </button>
      <button
        type="button"
        aria-label={`Remove ${name || email}`}
        disabled={disabled}
        className="flex size-5 shrink-0 items-center justify-center rounded text-secondary hover:bg-selection-inactive disabled:opacity-40"
        onClick={onRemove}
      >
        <X size={14} aria-hidden="true" />
      </button>
    </li>
  );
}

/** InviteesField lists invitees and adds addresses through compose's recipient input. */
export function InviteesField({ invitees, onChange, suggest, available, disabled }: InviteesFieldProps) {
  const locked = !available || Boolean(disabled);
  const invited = new Set(invitees.map(({ email }) => email.toLowerCase()));
  const add = (addresses: Address[]) => {
    const seen = new Set(invited);
    const next = [...invitees];
    for (const { address, name } of addresses) {
      if (seen.has(address.toLowerCase())) continue;
      seen.add(address.toLowerCase());
      next.push({ email: address, name, role: "required" });
    }
    if (next.length !== invitees.length) onChange(next);
  };
  return (
    <div>
      {invitees.length > 0 && (
        <ul aria-label="Invitees">
          {invitees.map((invitee, index) => (
            <InviteeRow
              key={invitee.email.toLowerCase()}
              invitee={invitee}
              disabled={locked}
              onToggle={() =>
                onChange(
                  invitees.map((item, position) =>
                    position === index ? { ...item, role: item.role === "optional" ? "required" : "optional" } : item,
                  ),
                )
              }
              onRemove={() => onChange(invitees.filter((_, position) => position !== index))}
            />
          ))}
        </ul>
      )}
      {!locked && (
        <RecipientField
          label="Add Invitee"
          value={[]}
          onChange={add}
          suggest={async (prefix) =>
            (await suggest(prefix)).filter(({ address }) => !invited.has(address.toLowerCase()))
          }
        />
      )}
      {!available && (
        <p className="text-[12px] leading-4 text-tertiary">Invitations aren't available in this calendar.</p>
      )}
    </div>
  );
}
