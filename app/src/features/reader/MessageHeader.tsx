// The reader's per-message header, attachment strip and remote-content
// banner (docs/specs/ui.md, Reader). The header shows the sender's avatar,
// name, date, subject and recipients; the strip lists a message's
// attachments as chips; the banner reports remote content and trackers that
// were held back and offers to load them.
import { File, Star } from "lucide-react";
import { Fragment } from "react";

import { avatarTone, displayName, formatAddressList, formatHeaderDate, formatSize, initials } from "../../lib/format";
import type { Address, Message, Part } from "../../rpc/gen/api";

// Tailwind only generates a class it finds whole in the source, so the
// eight avatar tones are listed rather than built from the tone number.
export const AVATAR_CLASSES: Record<number, string> = {
  0: "bg-avatar-0",
  1: "bg-avatar-1",
  2: "bg-avatar-2",
  3: "bg-avatar-3",
  4: "bg-avatar-4",
  5: "bg-avatar-5",
  6: "bg-avatar-6",
  7: "bg-avatar-7",
};

/** MessageHeaderProps are a header's inputs. */
export interface MessageHeaderProps {
  message: Message;
  vip?: boolean;
  onAddress?: (address: Address, at: { x: number; y: number }) => void;
}

function AddressButton({
  address,
  onAddress,
  className,
}: {
  address: Address;
  onAddress: NonNullable<MessageHeaderProps["onAddress"]>;
  className?: string;
}) {
  return (
    <button
      type="button"
      aria-label={address.address}
      title={address.address}
      className={className}
      onClick={(event) => {
        const rect = event.currentTarget.getBoundingClientRect();
        onAddress(address, { x: rect.left, y: rect.bottom });
      }}
    >
      {displayName(address)}
    </button>
  );
}

function AddressLine({
  label,
  addresses,
  onAddress,
}: {
  label: string;
  addresses: Address[];
  onAddress: MessageHeaderProps["onAddress"];
}) {
  if (addresses.length === 0) return null;
  return (
    <div className="text-reader-meta text-secondary truncate">
      {`${label}: `}
      {onAddress ? (
        <>
          {addresses.slice(0, 3).map((address, index) => (
            <Fragment key={`${address.address}:${address.name}`}>
              {index > 0 && ", "}
              <AddressButton address={address} onAddress={onAddress} />
            </Fragment>
          ))}
          {addresses.length > 3 && ` & ${addresses.length - 3} more`}
        </>
      ) : (
        formatAddressList(addresses)
      )}
    </div>
  );
}

/** MessageHeader shows the sender, subject, recipients and date. */
export function MessageHeader(props: MessageHeaderProps) {
  const { summary, to, cc } = props.message;
  const subject = summary.subject.trim();

  return (
    <header className="flex gap-3 px-5 py-4">
      <div
        aria-hidden="true"
        className={`size-10 shrink-0 rounded-full flex items-center justify-center text-[13px] font-semibold text-white ${
          AVATAR_CLASSES[avatarTone(summary.from.address)] ?? ""
        }`}
      >
        {initials(summary.from)}
      </div>
      <div className="min-w-0 flex-1 select-text">
        <div className="flex items-baseline gap-2">
          <span className="flex min-w-0 flex-1 items-center gap-1">
            {props.onAddress ? (
              <AddressButton
                address={summary.from}
                onAddress={props.onAddress}
                className="text-reader-sender truncate text-left"
              />
            ) : (
              <span className="text-reader-sender truncate" title={summary.from.address}>
                {displayName(summary.from)}
              </span>
            )}
            {props.vip && (
              <Star size={12} role="img" aria-label="VIP" className="shrink-0 fill-current text-secondary" />
            )}
          </span>
          <span className="text-reader-meta text-secondary shrink-0">{formatHeaderDate(new Date(summary.date))}</span>
        </div>
        <div className="text-reader-subject">{subject === "" ? "(No Subject)" : subject}</div>
        <AddressLine label="To" addresses={to} onAddress={props.onAddress} />
        <AddressLine label="Cc" addresses={cc} onAddress={props.onAddress} />
      </div>
    </header>
  );
}

/** AttachmentStripProps are the attachment strip's inputs. */
export interface AttachmentStripProps {
  parts: Part[];
  onOpen: (part: Part) => void;
}

/** isAttachment is whether a part is shown as an attachment rather than inline. */
function isAttachment(part: Part): boolean {
  return part.disposition === "attachment" || (part.filename !== "" && part.disposition !== "inline");
}

/** AttachmentStrip shows a message's attachments as chips. */
export function AttachmentStrip(props: AttachmentStripProps) {
  const attachments = props.parts.filter(isAttachment);
  if (attachments.length === 0) {
    return null;
  }

  return (
    <ul aria-label="Attachments" className="flex flex-wrap gap-2 px-5 pb-4">
      {attachments.map((part) => (
        <li key={part.path}>
          <button
            type="button"
            title={part.filename}
            className="flex h-8 items-center gap-2 rounded-md bg-banner px-2"
            onClick={() => props.onOpen(part)}
          >
            <File size={16} className="text-secondary" />
            <span className="max-w-[200px] truncate text-[12px]">
              {part.filename === "" ? "Untitled" : part.filename}
            </span>
            <span className="text-[12px] text-secondary tabular-nums">{formatSize(part.size)}</span>
          </button>
        </li>
      ))}
    </ul>
  );
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
export function RemoteBanner(props: RemoteBannerProps) {
  const { remote, trackers, loading, onLoad } = props;
  if (remote === 0 && trackers === 0) {
    return null;
  }

  const sentences: string[] = [];
  if (remote > 0) {
    sentences.push("This message contains remote content.");
  }
  if (trackers > 0) {
    sentences.push(`${trackers} tracker${trackers === 1 ? "" : "s"} blocked.`);
  }

  return (
    <div role="status" className="flex items-center gap-2 bg-banner px-5 py-2 text-[12px]">
      <span>{sentences.join(" ")}</span>
      {remote > 0 && (
        <button
          type="button"
          className="ml-auto h-6 shrink-0 whitespace-nowrap rounded border border-separator bg-window px-2"
          disabled={loading}
          onClick={onLoad}
        >
          {loading ? "Loading…" : "Load Remote Content"}
        </button>
      )}
    </div>
  );
}

/** ReminderBanner offers to change or clear a pending reminder. */
export function ReminderBanner(props: { when: string; disabled?: boolean; onChange: () => void; onClear: () => void }) {
  return (
    <div role="status" className="flex items-center gap-2 bg-banner px-5 py-2 text-[12px]">
      <span>{`Remind Me: ${props.when}`}</span>
      <button
        type="button"
        className="ml-auto h-6 shrink-0 whitespace-nowrap rounded border border-separator bg-window px-2 disabled:opacity-40"
        disabled={props.disabled}
        onClick={props.onChange}
      >
        Change…
      </button>
      <button
        type="button"
        className="h-6 shrink-0 whitespace-nowrap rounded border border-separator bg-window px-2 disabled:opacity-40"
        disabled={props.disabled}
        onClick={props.onClear}
      >
        Clear
      </button>
    </div>
  );
}
