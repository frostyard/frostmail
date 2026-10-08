// The reader: the selected message's conversation (docs/design/app.md, Reader).
import { invoke, isTauri } from "@tauri-apps/api/core";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { MessageFrame } from "../features/reader/MessageFrame";
import { AttachmentStrip, MessageHeader, RemoteBanner } from "../features/reader/MessageHeader";
import { PlainText } from "../features/reader/PlainText";
import type { Message, MessageSummary, Part, Rendering } from "../rpc/gen/api";

/** MAX_CONVERSATION is how many messages the reader shows before "Show earlier". */
export const MAX_CONVERSATION = 20;

/** ReaderHandle lets the window page the reader. */
export interface ReaderHandle {
  page: (direction: 1 | -1) => void;
  focus: () => void;
}

/** openLink opens a URL in the system browser. */
export function openLink(url: string): void {
  if (isTauri()) void invoke("open_link", { url }).catch(() => {});
  else window.open(url, "_blank", "noopener,noreferrer");
}

/** ReaderContainer shows the selection in the reader pane. */
export const ReaderContainer = forwardRef<ReaderHandle>(function ReaderContainer(_props, ref) {
  const { selected, setFocus } = useUI();
  const scroller = useRef<HTMLElement>(null);
  useImperativeHandle(ref, () => ({
    page: (direction) => {
      const el = scroller.current;
      if (el) el.scrollBy({ top: direction * el.clientHeight * 0.9 });
    },
    focus: () => scroller.current?.focus(),
  }));
  const id = selected.length === 1 ? selected[0] : undefined;
  return (
    <section
      ref={scroller}
      // biome-ignore lint/a11y/noNoninteractiveTabindex: the reader is a focusable, scrollable pane.
      tabIndex={0}
      aria-label="Message"
      onFocus={() => setFocus("reader")}
      className="h-full overflow-y-auto bg-window outline-none"
    >
      {selected.length === 0 && <Empty text="No Message Selected" />}
      {selected.length > 1 && <Empty text={`${selected.length} Messages Selected`} />}
      {id !== undefined && <Conversation key={id} id={id} />}
    </section>
  );
});

function Empty({ text }: { text: string }) {
  return <div className="flex h-full items-center justify-center text-empty text-secondary">{text}</div>;
}

function Conversation({ id }: { id: number }) {
  const client = useClient();
  // Only the Trash mailboxes matter here; counts change on every flag change.
  const trashKey = useMail((s) =>
    s.mailboxes
      .filter((mb) => mb.role === "trash")
      .map((mb) => mb.id)
      .join(","),
  );
  const [items, setItems] = useState<MessageSummary[] | null>(null);
  const [showAll, setShowAll] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [s] = await client.message.summaries({ ids: [id] });
      if (!s || cancelled) return;
      let rows = [s];
      if (s.threadId !== 0) rows = await client.thread.messages({ id: s.threadId });
      const trash = new Set(trashKey === "" ? [] : trashKey.split(",").map(Number));
      const inTrash = (r: MessageSummary) => r.mailboxIds.some((mb) => trash.has(mb));
      rows = rows.filter((r) => r.id === id || !inTrash(r) || inTrash(s)).reverse();
      if (cancelled) return;
      setItems(rows);
      // What the reader shows is read (docs/specs/ui.md, Behavior).
      const unseen = rows
        .slice(0, MAX_CONVERSATION)
        .filter((r) => !r.flags.seen)
        .map((r) => r.id);
      if (unseen.length > 0) void client.message.setFlags({ ids: unseen, changes: { seen: true } }).catch(() => {});
    })().catch((err: unknown) => {
      if (!cancelled) setError(err instanceof Error ? err.message : String(err));
    });
    return () => {
      cancelled = true;
    };
  }, [client, id, trashKey]);

  if (error) return <Empty text={error} />;
  if (!items) return null;
  const shown = showAll ? items : items.slice(0, MAX_CONVERSATION);
  return (
    <div className="divide-y divide-separator">
      {shown.map((s) => (
        <ConversationMessage key={s.id} summary={s} />
      ))}
      {shown.length < items.length && (
        <button
          type="button"
          className="w-full py-3 text-[12px] text-accent"
          onClick={() => setShowAll(true)}
        >{`Show ${items.length - shown.length} Earlier Messages`}</button>
      )}
    </div>
  );
}

function ConversationMessage({ summary }: { summary: MessageSummary }) {
  const client = useClient();
  const [message, setMessage] = useState<Message | null>(null);
  const [rendering, setRendering] = useState<Rendering | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadingRemote, setLoadingRemote] = useState(false);
  const id = summary.id;

  useEffect(() => {
    let cancelled = false;
    const fail = (err: unknown) => {
      if (!cancelled) setError(err instanceof Error ? err.message : String(err));
    };
    client.message.get({ id }).then((m) => !cancelled && setMessage(m), fail);
    client.message.render({ id }).then((r) => !cancelled && setRendering(r), fail);
    return () => {
      cancelled = true;
    };
  }, [client, id]);

  const loadRemote = useCallback(() => {
    setLoadingRemote(true);
    client.message
      .render({ id, remote: true })
      .then(setRendering, () => {})
      .finally(() => setLoadingRemote(false));
  }, [client, id]);

  const openPart = useCallback(
    (part: Part) => {
      void client.message.part({ id, path: part.path }).then((f) => {
        if (isTauri()) void invoke("open_part", { path: f.path }).catch(() => {});
        else window.open(`/mailpart/${f.path}`, "_blank", "noopener,noreferrer");
      });
    },
    [client, id],
  );

  return (
    <article aria-label={summary.subject} className="pb-2">
      {message && <MessageHeader message={message} />}
      {rendering && (
        <RemoteBanner
          remote={rendering.remote}
          trackers={rendering.trackers}
          loading={loadingRemote}
          onLoad={loadRemote}
        />
      )}
      {error && <p className="px-5 py-4 text-reader-meta text-secondary">{error}</p>}
      {rendering &&
        (rendering.html !== "" ? (
          <MessageFrame html={rendering.html} onOpenLink={openLink} />
        ) : (
          <PlainText text={rendering.text} onOpenLink={openLink} />
        ))}
      {message && <AttachmentStrip parts={message.parts} onOpen={openPart} />}
    </article>
  );
}
