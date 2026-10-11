// Sending feedback in the main window (docs/specs/compose-ui.md, Undo toast
// and Outbox): a toast with Undo for each message still in its undo delay,
// and the Outbox section listing messages not yet sent.
import { useEffect, useState } from "react";

import { useClient } from "../data/session";
import { useMail } from "../data/stores";
import { TimeSheet } from "../features/later/TimeSheet";
import { isSendLater, OutboxStatus, SendLaterStatus, UndoToast } from "../features/outbox/Outbox";
import { appLocale } from "../lib/calendarDates";
import type { Client, OutboxItem } from "../rpc/gen/api";
import { openCompose } from "./compose";

/** useNow returns the time, updated every intervalMs while active. */
function useNow(intervalMs: number, active: boolean): Date {
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    if (!active) return;
    setNow(new Date());
    const t = setInterval(() => setNow(new Date()), intervalMs);
    return () => clearInterval(t);
  }, [intervalMs, active]);
  return now;
}

/** secondsLeft is how long a queued message still waits, rounded up. */
function secondsLeft(item: OutboxItem, now: Date): number {
  if (item.scheduled || item.state !== "queued" || item.attempts > 0 || item.sendAt === undefined) return 0;
  return Math.ceil((Date.parse(item.sendAt) - now.getTime()) / 1000);
}

/** undo cancels a queued message and reopens its draft, when it has one. */
async function undo(client: Client, id: number): Promise<void> {
  const draft = await client.outbox.cancel({ id });
  if (draft.id !== 0) await openCompose(draft.id);
}

/** UndoToasts stacks a toast for every message still in its undo delay, newest on top. */
export function UndoToasts() {
  const client = useClient();
  const outbox = useMail((s) => s.outbox);
  const waiting = outbox.some((i) => !i.scheduled && i.state === "queued" && i.attempts === 0);
  const now = useNow(250, waiting);
  const toasts = outbox
    .map((item) => ({ item, left: secondsLeft(item, now) }))
    .filter((t) => t.left > 0)
    .sort((a, b) => b.item.id - a.item.id);
  if (toasts.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-4 left-1/2 z-20 flex -translate-x-1/2 flex-col items-center gap-2">
      {toasts.map(({ item, left }) => (
        <div key={item.id} className="pointer-events-auto">
          <UndoToast
            subject={item.subject}
            secondsLeft={left}
            onUndo={() => void undo(client, item.id).catch((err: unknown) => console.warn("undo send", err))}
          />
        </div>
      ))}
    </div>
  );
}

/** SendLaterSection connects scheduled messages and the time sheet to the outbox. */
export function SendLaterSection() {
  const client = useClient();
  const outbox = useMail((s) => s.outbox);
  const [changing, setChanging] = useState<OutboxItem | null>(null);
  const now = useNow(30_000, outbox.some(isSendLater));
  const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const locale = appLocale(navigator.language);
  const reschedule = (item: OutboxItem, at: Date) => {
    void client.outbox
      .reschedule({ id: item.id, sendAt: at.toISOString() })
      .catch((err: unknown) => console.warn("reschedule", err));
  };
  return (
    <>
      <SendLaterStatus
        items={outbox}
        now={now}
        timeZone={timeZone}
        locale={locale}
        onEdit={(item) => void undo(client, item.id).catch((err: unknown) => console.warn("edit", err))}
        onSendNow={(item) => reschedule(item, new Date())}
        onChangeTime={setChanging}
      />
      {changing && (
        <TimeSheet
          key={changing.id}
          title="Send Later"
          initial={new Date(changing.sendAt ?? now.toISOString())}
          now={now}
          timeZone={timeZone}
          onChoose={(at) => {
            reschedule(changing, at);
            setChanging(null);
          }}
          onCancel={() => setChanging(null)}
        />
      )}
    </>
  );
}

/** OutboxSection is the sidebar's Outbox: messages not yet sent, with Retry and Edit. */
export function OutboxSection() {
  const client = useClient();
  const outbox = useMail((s) => s.outbox);
  const now = useNow(30_000, outbox.length > 0);
  return (
    <OutboxStatus
      items={outbox}
      now={now}
      onRetry={(id) => void client.outbox.retry({ id }).catch((err: unknown) => console.warn("retry", err))}
      onEdit={(item) => {
        const open =
          item.state === "queued"
            ? undo(client, item.id)
            : item.draftId !== undefined
              ? openCompose(item.draftId)
              : Promise.resolve();
        void open.catch((err: unknown) => console.warn("edit", err));
      }}
    />
  );
}
