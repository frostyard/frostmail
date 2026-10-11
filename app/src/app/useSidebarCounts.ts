// Batched message counts for the sidebar's built-in sources.
import { useEffect, useState } from "react";

import { useClient } from "../data/session";
import { sourceQuery } from "../data/stores";
import type { SidebarCounts, VipGroup } from "../lib/mailboxTree";
import type { ViewCount, ViewQuery } from "../rpc/gen/api";

/** countQueries lists VIPs, VIP groups, Flagged and the seven colors. */
export function countQueries(groups: VipGroup[]): ViewQuery[] {
  return [
    sourceQuery({ kind: "vips" }, false),
    ...groups.map((group) => sourceQuery({ kind: "vip", key: group.key, addresses: group.addresses }, false)),
    sourceQuery({ kind: "flagged" }, false),
    ...Array.from({ length: 7 }, (_, index) => sourceQuery({ kind: "flagColor", color: index + 1 }, false)),
  ];
}

/** toCounts maps batched answers to sidebar rows, defaulting missing answers to zero. */
export function toCounts(groups: VipGroup[], results: ViewCount[]): SidebarCounts {
  const at = (index: number): ViewCount => results[index] ?? { total: 0, unread: 0 };
  return {
    vips: at(0),
    vip: Object.fromEntries(groups.map((group, index) => [group.key, at(index + 1)])),
    flagged: at(groups.length + 1),
    colors: Array.from({ length: 7 }, (_, index) => at(groups.length + 2 + index)),
  };
}

/** useSidebarCounts refreshes counts on group changes and debounced maild events. */
export function useSidebarCounts(groups: VipGroup[]): SidebarCounts | undefined {
  const client = useClient();
  const [counts, setCounts] = useState<SidebarCounts>();
  const groupKey = JSON.stringify(groups);
  useEffect(() => {
    const current: VipGroup[] = JSON.parse(groupKey);
    let stopped = false;
    let request = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = () => {
      const version = ++request;
      void client.view
        .count({ queries: countQueries(current) })
        .then((results) => {
          if (!stopped && version === request) setCounts(toCounts(current, results));
        })
        .catch(() => {});
    };
    const off = client.transport.onEvent((event) => {
      if (["mailbox.changed", "vip.changed", "settings.changed"].includes(event.event)) {
        clearTimeout(timer);
        timer = setTimeout(refresh, 300);
      }
    });
    refresh();
    return () => {
      stopped = true;
      clearTimeout(timer);
      off();
    };
  }, [client, groupKey]);
  return counts;
}
