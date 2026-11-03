// The sidebar's Follow Up data: the conversations waiting on other people
// (docs/specs/ui.md, Sidebar), kept fresh as maild changes.
import { useEffect, useState } from "react";

import { useClient } from "../data/session";
import { FOLLOW_UP_DAYS, FOLLOW_UP_LIMIT } from "../features/followup/FollowUp";
import type { Awaiting } from "../rpc/gen/api";

/** FOLLOW_UP_REFRESH_MS is how often the section asks again with no maild event. */
const FOLLOW_UP_REFRESH_MS = 15 * 60_000;

/** FOLLOW_UP_DEBOUNCE_MS is how long it waits after the last maild event. */
const FOLLOW_UP_DEBOUNCE_MS = 1000;

/** FOLLOW_UP_EVENTS are the events that can change who is waiting on a reply. */
const FOLLOW_UP_EVENTS = [
  "mailbox.changed",
  "message.changed",
  "message.removed",
  "vip.changed",
  "account.changed",
  "settings.changed",
];

/** useFollowUp loads the threads waiting on other people when the client
 *  connects, after debounced maild events, and every 15 minutes. */
export function useFollowUp(): Awaiting | undefined {
  const client = useClient();
  const [awaiting, setAwaiting] = useState<Awaiting>();
  useEffect(() => {
    let stopped = false;
    let request = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = () => {
      const version = ++request;
      void client.thread
        .awaitingOthers({ limit: FOLLOW_UP_LIMIT, waitingDays: FOLLOW_UP_DAYS })
        .then((next) => {
          if (!stopped && version === request) setAwaiting(next);
        })
        .catch((err: unknown) => {
          // The last answer stays: a failed refresh is not an empty section.
          if (!stopped) console.warn("follow up", err);
        });
    };
    const off = client.transport.onEvent((event) => {
      if (!FOLLOW_UP_EVENTS.includes(event.event)) return;
      clearTimeout(timer);
      timer = setTimeout(refresh, FOLLOW_UP_DEBOUNCE_MS);
    });
    refresh();
    const every = setInterval(refresh, FOLLOW_UP_REFRESH_MS);
    return () => {
      stopped = true;
      clearTimeout(timer);
      clearInterval(every);
      off();
    };
  }, [client]);
  return awaiting;
}
