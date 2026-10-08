// Turning typed recipient text into addresses (docs/design/send.md, Compose
// windows). Text is split on commas, semicolons and newlines except inside
// double quotes, so a quoted name such as "Smith, Bob" stays one piece. A
// piece of the form name <addr> gives its name and address; a bare piece is
// just the address. Pieces that do not parse are reported trimmed, and a
// repeated address is dropped case-insensitively, keeping the first.
import type { Address } from "../rpc/gen/api";

/** ParsedAddresses is the result of parseAddresses. */
export interface ParsedAddresses {
  /** Addresses that parsed, in order. */
  addresses: Address[];
  /** Pieces that did not parse, trimmed, in order. */
  invalid: string[];
}

const ANGLE_FORM = /^(.*)<([^<>]*)>$/;
const ADDRESS_FORM = /^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/;

/** parseAddresses splits typed or pasted text into addresses, in order, dropping repeated addresses case-insensitively. */
export function parseAddresses(text: string): ParsedAddresses {
  const addresses: Address[] = [];
  const invalid: string[] = [];
  const seen = new Set<string>();
  for (const raw of splitPieces(text)) {
    const piece = raw.trim();
    if (piece === "") continue;
    const addr = addressOf(piece);
    if (addr === undefined) {
      invalid.push(piece);
      continue;
    }
    const key = addr.address.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    addresses.push(addr);
  }
  return { addresses, invalid };
}

/** isValidAddress reports whether text is a plausible email address: a local part, then a domain of at least two dot-separated labels. */
export function isValidAddress(text: string): boolean {
  return ADDRESS_FORM.test(text);
}

function addressOf(piece: string): Address | undefined {
  const angle = ANGLE_FORM.exec(piece);
  if (angle) {
    const address = (angle[2] ?? "").trim();
    if (!isValidAddress(address)) return undefined;
    return { name: unquote((angle[1] ?? "").trim()), address };
  }
  if (!isValidAddress(piece)) return undefined;
  return { name: "", address: piece };
}

function splitPieces(text: string): string[] {
  const pieces: string[] = [];
  let current = "";
  let inQuotes = false;
  for (const ch of text) {
    if (ch === '"') {
      inQuotes = !inQuotes;
      current += ch;
    } else if (!inQuotes && (ch === "," || ch === ";" || ch === "\n" || ch === "\r")) {
      pieces.push(current);
      current = "";
    } else {
      current += ch;
    }
  }
  pieces.push(current);
  return pieces;
}

function unquote(name: string): string {
  if (name.length >= 2 && name.startsWith('"') && name.endsWith('"')) {
    return name.slice(1, -1);
  }
  return name;
}
