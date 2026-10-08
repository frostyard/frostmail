// One server's settings in the account form (docs/specs/settings-ui.md,
// ServerFields). Task T-0056 implements it; the stubs draw nothing.
import type { ServerConfig, TLSMode } from "../../rpc/gen/api";

/** ServerFieldsProps are the server fields' inputs. */
export interface ServerFieldsProps {
  /** The inputs' ID prefix, which also picks the default ports. */
  id: "imap" | "smtp";
  /** The fieldset's legend, such as "Incoming Mail (IMAP)". */
  legend: string;
  value: ServerConfig;
  onChange: (v: ServerConfig) => void;
  disabled?: boolean;
}

/** defaultPort is the usual port of a server kind over a security mode. */
export function defaultPort(_id: "imap" | "smtp", _tls: TLSMode): number {
  return 0;
}

/** ServerFields edits one server's host, port, security and user name. */
export function ServerFields(_props: ServerFieldsProps) {
  return null;
}
