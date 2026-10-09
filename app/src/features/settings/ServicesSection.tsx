// An account's contacts, calendar and tasks services in the settings
// window (docs/specs/settings-ui.md, ServicesSection).
import { useId, useState } from "react";

import type { ServiceKind, ServiceSettings } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, LABEL } from "./labels";

/** ServicesSectionProps are the services section's inputs. */
export interface ServicesSectionProps {
  /** account.services: contacts, calendar, tasks. */
  services: ServiceSettings[];
  /** The service a request is running for. */
  busy: ServiceKind | null;
  /** Each service's last request error. */
  errors: Partial<Record<ServiceKind, string>>;
  now: Date;
  onToggle: (service: ServiceKind, enabled: boolean, url?: string) => void;
  onSignIn: () => void;
}

/** ServicesSection turns an account's services on and off. */
export function ServicesSection(props: ServicesSectionProps) {
  const heading = useId();
  const available = props.services.filter((service) => service.available);
  if (available.length === 0) {
    return (
      <p className="text-[12px] leading-4 text-secondary">
        This account has no contacts, calendars or tasks Frostmail can reach.
      </p>
    );
  }
  return (
    <section aria-labelledby={heading}>
      <h3 id={heading} className="mt-5 mb-2 text-[13px] leading-[18px] font-semibold">
        Contacts, Calendars and Tasks
      </h3>
      {available.map((service) => (
        <ServiceRow key={service.service} {...props} value={service} />
      ))}
    </section>
  );
}

const NAMES: Record<ServiceKind, string> = { contacts: "Contacts", calendar: "Calendars", tasks: "Tasks" };

function ServiceStatus({ value, busy, now, onSignIn, errors }: ServicesSectionProps & { value: ServiceSettings }) {
  const style = "text-[12px] leading-4";
  if (busy === value.service) return <span className={`${style} text-secondary`}>Connecting…</span>;
  if (!value.enabled) return null;
  if (!value.signedIn)
    return (
      <>
        <span className={`${style} text-flag-1`}>Sign in to Google again to allow this.</span>
        <button type="button" className={BUTTON} onClick={onSignIn}>
          Sign In…
        </button>
      </>
    );
  const error = errors[value.service] ?? value.error;
  if (error) return <span className={`${style} text-flag-1`}>{error}</span>;
  let status = "Waiting for the first sync";
  if (value.lastSyncAt) {
    const synced = new Date(value.lastSyncAt);
    status =
      synced.toDateString() === now.toDateString()
        ? `Synced at ${synced.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" })}`
        : `Synced ${synced.toLocaleDateString(undefined, { year: "2-digit", month: "numeric", day: "numeric" })}`;
  }
  return <span className={`${style} text-secondary`}>{status}</span>;
}

function ServiceRow(props: ServicesSectionProps & { value: ServiceSettings }) {
  const { value, busy, errors, onToggle } = props;
  const [url, setURL] = useState("");
  const id = `service-url-${value.service}`;
  const connecting = busy === value.service;
  return (
    <div>
      <div className="flex items-center gap-3">
        <label className="flex items-center gap-2 text-[13px] leading-[18px]">
          <input
            type="checkbox"
            checked={value.enabled}
            disabled={connecting}
            onChange={(event) => onToggle(value.service, event.target.checked)}
          />
          {NAMES[value.service]}
        </label>
        <ServiceStatus {...props} />
      </div>
      {!value.enabled && errors[value.service] && (
        <>
          <p role="alert" className={ALERT}>
            {errors[value.service]}
          </p>
          <div className="flex items-center gap-3">
            <label htmlFor={id} className={LABEL}>
              Server:
            </label>
            <input
              id={id}
              type="url"
              className={FIELD}
              placeholder="https://dav.example.com/"
              value={url}
              onChange={(event) => setURL(event.target.value)}
            />
            <button
              type="button"
              className={BUTTON}
              disabled={url.trim() === "" || connecting}
              onClick={() => onToggle(value.service, true, url.trim())}
            >
              Connect
            </button>
          </div>
        </>
      )}
    </div>
  );
}
