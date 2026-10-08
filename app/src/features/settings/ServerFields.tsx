// One server's settings in the account form (docs/specs/settings-ui.md,
// ServerFields): the host, port, security mode and user name of one IMAP
// or SMTP server, laid out as a labeled grid that reports every edit as a
// new ServerConfig and moves a default port along with the security mode.
import type { ChangeEvent } from "react";

import type { ServerConfig, TLSMode } from "../../rpc/gen/api";
import { FIELD, GRID, LABEL, TLS_LABEL, TLS_MODES } from "./labels";

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

const DEFAULT_PORTS: Record<"imap" | "smtp", Record<TLSMode, number>> = {
  imap: { tls: 993, starttls: 143, insecure: 143 },
  smtp: { tls: 465, starttls: 587, insecure: 25 },
};

/** defaultPort is the usual port of a server kind over a security mode. */
export function defaultPort(id: "imap" | "smtp", tls: TLSMode): number {
  return DEFAULT_PORTS[id][tls];
}

function withTls(id: "imap" | "smtp", value: ServerConfig, tls: TLSMode): ServerConfig {
  const wasDefault = value.port === 0 || value.port === defaultPort(id, value.tls);
  return { ...value, tls, port: wasDefault ? defaultPort(id, tls) : value.port };
}

/** SecuritySelect is one server's security mode dropdown. */
function SecuritySelect(props: { id: string; value: TLSMode; onChange: (tls: TLSMode) => void }) {
  return (
    <select
      id={`${props.id}-tls`}
      className={FIELD}
      value={props.value}
      onChange={(event: ChangeEvent<HTMLSelectElement>) => {
        const tls = TLS_MODES.find((mode) => mode === event.target.value);
        if (tls) props.onChange(tls);
      }}
    >
      {TLS_MODES.map((mode) => (
        <option key={mode} value={mode}>
          {TLS_LABEL[mode]}
        </option>
      ))}
    </select>
  );
}

/** ServerFields edits one server's host, port, security and user name. */
export function ServerFields(props: ServerFieldsProps) {
  const { id, legend, value, onChange, disabled } = props;
  return (
    <fieldset className={GRID} disabled={disabled}>
      <legend className="mb-2 text-[13px] leading-[18px] font-semibold">{legend}</legend>
      <label className={LABEL} htmlFor={`${id}-host`}>
        Server:
      </label>
      <input
        id={`${id}-host`}
        type="text"
        className={FIELD}
        autoComplete="off"
        spellCheck={false}
        value={value.host}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onChange({ ...value, host: event.target.value })}
      />
      <label className={LABEL} htmlFor={`${id}-port`}>
        Port:
      </label>
      <input
        id={`${id}-port`}
        type="number"
        min={1}
        max={65535}
        className={`${FIELD} w-24`}
        value={value.port === 0 ? "" : String(value.port)}
        onChange={(event: ChangeEvent<HTMLInputElement>) =>
          onChange({ ...value, port: Number.parseInt(event.target.value, 10) || 0 })
        }
      />
      <label className={LABEL} htmlFor={`${id}-tls`}>
        Security:
      </label>
      <SecuritySelect id={id} value={value.tls} onChange={(tls) => onChange(withTls(id, value, tls))} />
      <label className={LABEL} htmlFor={`${id}-username`}>
        User Name:
      </label>
      <input
        id={`${id}-username`}
        type="text"
        className={FIELD}
        autoComplete="off"
        spellCheck={false}
        value={value.username}
        onChange={(event: ChangeEvent<HTMLInputElement>) => onChange({ ...value, username: event.target.value })}
      />
    </fieldset>
  );
}
