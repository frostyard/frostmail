// Turning typed recipient text into addresses (docs/design/send.md, Compose
// windows). Task T-0042 implements both functions; the stubs find nothing.
import type { Address } from "../rpc/gen/api";

/** ParsedAddresses is the result of parseAddresses. */
export interface ParsedAddresses {
  /** Addresses that parsed, in order. */
  addresses: Address[];
  /** Pieces that did not parse, trimmed, in order. */
  invalid: string[];
}

/** parseAddresses splits typed or pasted text into addresses. */
export function parseAddresses(_text: string): ParsedAddresses {
  return { addresses: [], invalid: [] };
}

/** isValidAddress reports whether text is a plausible email address. */
export function isValidAddress(_text: string): boolean {
  return false;
}
