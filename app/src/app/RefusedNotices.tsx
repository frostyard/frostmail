import { useLayoutEffect, useRef, useState } from "react";
import { useClient } from "../data/session";
import { RefusedNotice, refusedText } from "../features/people/RefusedNotice";

/** NOTICE_MS is how long a refused change stays visible. */
export const NOTICE_MS = 8_000;

/** RefusedNotices stacks contact and event refusals until dismissed or expired. */
export function RefusedNotices() {
  const client = useClient();
  const [notices, setNotices] = useState<{ id: number; text: string }[]>([]);
  const nextId = useRef(0);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());
  const dismiss = (id: number) => {
    clearTimeout(timers.current.get(id));
    timers.current.delete(id);
    setNotices((current) => current.filter((notice) => notice.id !== id));
  };
  // Subscribe at commit, as the window's other handlers do: a refusal
  // that comes as soon as the window shows is not missed.
  useLayoutEffect(() => {
    const pending = timers.current;
    const off = client.transport.onEvent((event) => {
      if (event.event !== "people.refused" && event.event !== "calendar.refused") return;
      const name = event.event === "people.refused" ? event.data.name : event.data.summary;
      const id = nextId.current++;
      setNotices((current) => [...current, { id, text: refusedText(name, event.data.reason) }]);
      pending.set(
        id,
        setTimeout(() => {
          pending.delete(id);
          setNotices((current) => current.filter((notice) => notice.id !== id));
        }, NOTICE_MS),
      );
    });
    return () => {
      off();
      for (const timer of pending.values()) clearTimeout(timer);
      pending.clear();
    };
  }, [client]);
  if (notices.length === 0) return null;
  return (
    <div className="pointer-events-none fixed bottom-4 left-1/2 z-20 flex -translate-x-1/2 flex-col items-center gap-2">
      {notices.map((notice) => (
        <div key={notice.id} className="pointer-events-auto">
          <RefusedNotice text={notice.text} onDismiss={() => dismiss(notice.id)} />
        </div>
      ))}
    </div>
  );
}
