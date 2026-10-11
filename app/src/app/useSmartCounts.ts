import { useEffect, useState } from "react";

import { useClient } from "../data/session";
import type { SmartMailbox, ViewCount } from "../rpc/gen/api";

/** useSmartCounts refreshes smart mailbox counts after debounced maild events. */
export function useSmartCounts(smarts: SmartMailbox[]): Record<number, ViewCount> {
  const client = useClient();
  const [counts, setCounts] = useState<Record<number, ViewCount>>({});
  const idsKey = JSON.stringify(smarts.map((smart) => smart.id));
  useEffect(() => {
    const ids: number[] = JSON.parse(idsKey);
    let stopped = false;
    let request = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = () => {
      const version = ++request;
      if (ids.length === 0) {
        setCounts({});
        return;
      }
      void client.view
        .count({ queries: ids.map((smartMailboxId) => ({ smartMailboxId })) })
        .then((results) => {
          if (!stopped && version === request)
            setCounts(Object.fromEntries(ids.map((id, index) => [id, results[index] ?? { total: 0, unread: 0 }])));
        })
        .catch(() => {});
    };
    const off = client.transport.onEvent((event) => {
      if (["mailbox.changed", "vip.changed", "settings.changed", "smart.changed"].includes(event.event)) {
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
  }, [client, idsKey]);
  return counts;
}
