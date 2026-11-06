import { ShieldAlert } from "lucide-react";

/** SenderCheckBannerProps are the sender check banner's inputs. */
export interface SenderCheckBannerProps {
  /** The sender's address; empty when the message has none. */
  address: string;
  /** All Headers show. */
  showing: boolean;
  onToggle: () => void;
}

/** domainOf is an address's domain, lowercased; empty with none. */
function domainOf(address: string): string {
  const at = address.lastIndexOf("@");
  return at < 0
    ? ""
    : address
        .slice(at + 1)
        .trim()
        .toLowerCase();
}

/** SenderCheckBanner warns that the provider's sender checks failed (ADR-0050). */
export function SenderCheckBanner(props: SenderCheckBannerProps) {
  const domain = domainOf(props.address);
  return (
    <div role="status" className="flex items-center gap-2 bg-banner px-5 py-2 text-[12px]">
      <ShieldAlert size={14} aria-hidden="true" className="shrink-0 text-flag-1" />
      <span className="-ml-0.5">
        <span className="font-semibold">Sender not verified.</span>
        {domain
          ? ` This message says it is from ${domain}, but ${domain} did not vouch for it. Be careful with its links and attachments.`
          : " Its sender did not pass the provider's checks. Be careful with its links and attachments."}
      </span>
      <button
        type="button"
        className="ml-auto h-6 shrink-0 whitespace-nowrap rounded border border-separator bg-window px-2 print:hidden"
        onClick={props.onToggle}
      >
        {props.showing ? "Hide Details" : "Show Details"}
      </button>
    </div>
  );
}
