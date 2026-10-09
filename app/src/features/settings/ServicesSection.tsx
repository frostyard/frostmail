// An account's contacts, calendar and tasks services in the settings
// window (docs/specs/settings-ui.md, ServicesSection). Task T-0066 writes it.
import type { ServiceKind, ServiceSettings } from "../../rpc/gen/api";

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
export function ServicesSection(_props: ServicesSectionProps) {
  return null;
}
