import type { Vip } from "../rpc/gen/api";

/** vipSet builds a case-insensitive lookup of VIP addresses. */
export function vipSet(vips: Vip[]): ReadonlySet<string> {
  return new Set(vips.map((vip) => vip.address.toLowerCase()));
}

/** isVip matches an address without case or surrounding whitespace. */
export function isVip(address: string, set: ReadonlySet<string>): boolean {
  return set.has(address.trim().toLowerCase());
}
