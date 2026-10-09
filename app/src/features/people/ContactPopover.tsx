// The contact card mail shows for an address (docs/specs/pim-ui.md).
import { type ReactNode, useEffect, useId, useRef } from "react";
import { formatListDate } from "../../lib/format";
import type { Address, ContactCard } from "../../rpc/gen/api";
import { Avatar } from "./Avatar";

/** AddState is where Add to Contacts stands. */
export type AddState = "idle" | "adding" | "added" | { error: string };

/** ContactPopoverProps are the contact card's inputs. */
export interface ContactPopoverProps {
  /** The address the card was opened for; shown until card arrives. */
  address: Address;
  /** people.card's answer, or null while it is pending. */
  card: ContactCard | null;
  /** The person's photo as a data: URL, when loaded. */
  photo?: string;
  upcoming?: ReactNode;
  /** Where the card goes: below the clicked name, in window pixels. */
  at: { x: number; y: number };
  /** The clock for the recent mail's dates. */
  now: Date;
  add: AddState;
  onCompose: () => void;
  onAdd: () => void;
  onOpenPerson: (id: number) => void;
  onOpenMessage: (id: number) => void;
  onClose: () => void;
}

/** ContactPopover shows who an address is and what to do with them. */
export function ContactPopover(props: ContactPopoverProps) {
  const { address, card, photo, at, now, add, onCompose, onAdd, onOpenPerson, onOpenMessage, onClose } = props;
  const dialog = useRef<HTMLDivElement>(null);
  const recentId = useId();
  const name = card?.name.trim() || address.name.trim() || address.address;
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      onClose();
    };
    const outside = (event: MouseEvent) => {
      if (event.target instanceof Node && !dialog.current?.contains(event.target)) onClose();
    };
    document.addEventListener("keydown", keydown, true);
    document.addEventListener("mousedown", outside);
    return () => {
      document.removeEventListener("keydown", keydown, true);
      document.removeEventListener("mousedown", outside);
    };
  }, [onClose]);
  return (
    <div
      ref={dialog}
      role="dialog"
      aria-label={name}
      className="fixed z-50 w-[320px] rounded-[10px] border border-separator bg-window p-4 shadow-lg"
      style={{ left: Math.max(0, Math.min(at.x, window.innerWidth - 320)), top: at.y }}
    >
      <header className="flex items-start gap-3">
        <Avatar name={name} email={address.address} photo={photo} size={48} />
        <div className="min-w-0 select-text">
          <h2 className="text-[15px] font-semibold leading-5">{name}</h2>
          <div className="break-words text-[12px] leading-4 text-secondary">{address.address}</div>
          {card?.person?.organization && (
            <div className="text-[12px] leading-4 text-secondary">{card.person.organization}</div>
          )}
        </div>
      </header>
      <div className="mt-4 flex flex-wrap items-center gap-2 text-[12px]">
        <button type="button" className="h-7 rounded-md bg-accent px-2 text-accent-contrast" onClick={onCompose}>
          Message
        </button>
        {card?.canAdd && (add === "idle" || typeof add === "object") && (
          <button type="button" className="h-7 rounded-md border border-separator px-2" onClick={onAdd}>
            Add to Contacts
          </button>
        )}
        {add === "adding" && <span className="text-secondary">Adding…</span>}
        {add === "added" && <span className="text-secondary">Added</span>}
        {card?.person && (
          <button
            type="button"
            className="h-7 rounded-md border border-separator px-2"
            onClick={() => card.person && onOpenPerson(card.person.id)}
          >
            Open in People
          </button>
        )}
      </div>
      {typeof add === "object" && (
        <p role="alert" className="mt-2 text-[12px]">
          {add.error}
        </p>
      )}
      {card && card.recent.length > 0 && (
        <section aria-labelledby={recentId} className="mt-4">
          <h3 id={recentId} className="mb-2 text-sidebar-section text-secondary uppercase">
            Recent Mail
          </h3>
          {card.recent.map((message) => {
            const subject = message.subject.trim();
            return (
              <button
                key={message.id}
                type="button"
                className="flex h-8 w-full items-center gap-3 text-left hover:bg-selection-inactive"
                onClick={() => onOpenMessage(message.id)}
              >
                <span className={`min-w-0 flex-1 truncate text-list-subject ${subject ? "" : "text-tertiary"}`}>
                  {subject || "(No Subject)"}
                </span>
                <span className="shrink-0 text-list-date text-secondary tabular-nums">
                  {formatListDate(new Date(message.date), now)}
                </span>
              </button>
            );
          })}
        </section>
      )}
      {props.upcoming && <div className="mt-4">{props.upcoming}</div>}
    </div>
  );
}
