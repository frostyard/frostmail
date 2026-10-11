// The reader: the selected message's conversation (docs/design/app.md, Reader).
import { invoke, isTauri } from "@tauri-apps/api/core";
import { save } from "@tauri-apps/plugin-dialog";
import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";

import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { TimeSheet } from "../features/later/TimeSheet";
import { ContextMenu } from "../features/menu/ContextMenu";
import { type Answer, InvitationCard, invitationPart } from "../features/reader/InvitationCard";
import { MessageFrame } from "../features/reader/MessageFrame";
import {
  AttachmentStrip,
  MessageHeader,
  type MessageHeaderProps,
  ReminderBanner,
  RemoteBanner,
} from "../features/reader/MessageHeader";
import { PlainText } from "../features/reader/PlainText";
import { RawSourceSheet } from "../features/reader/RawSourceSheet";
import { UnsubscribeBanner } from "../features/reader/UnsubscribeBanner";
import { zoned } from "../lib/calendarDates";
import { whenText } from "../lib/later";
import { isVip, vipSet } from "../lib/vips";
import type {
  Address,
  Invitation,
  Message,
  MessageSource,
  MessageSummary,
  Part,
  Rendering,
  Unsubscribe,
  UnsubscribeResult,
} from "../rpc/gen/api";
import { ContactCardContainer } from "./ContactCardContainer";
import { remindMessages } from "./commands";
import { useCalendarFrame } from "./useCalendar";

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
  const [contact, setContact] = useState<{ address: Address; at: { x: number; y: number } } | null>(null);
  const openContact = useCallback((address: Address, at: { x: number; y: number }) => setContact({ address, at }), []);
  const closeContact = useCallback(() => setContact(null), []);
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
      data-print-area
      onFocus={() => setFocus("reader")}
      className="h-full overflow-y-auto bg-window outline-none"
    >
      {selected.length === 0 && <Empty text="No Message Selected" />}
      {selected.length > 1 && <Empty text={`${selected.length} Messages Selected`} />}
      {id !== undefined && <Conversation key={id} id={id} onAddress={openContact} />}
      {contact && <ContactCardContainer key={contact.address.address} {...contact} onClose={closeContact} />}
    </section>
  );
});

function Empty({ text }: { text: string }) {
  return <div className="flex h-full items-center justify-center text-empty text-secondary">{text}</div>;
}

function Conversation({ id, onAddress }: { id: number; onAddress: MessageHeaderProps["onAddress"] }) {
  const client = useClient();
  // Only the Trash mailboxes matter here; counts change on every flag change.
  const trashKey = useMail((s) =>
    s.mailboxes
      .filter((mb) => mb.role === "trash")
      .map((mb) => mb.id)
      .join(","),
  );
  // Read-only accounts are never marked read (docs/design/accounts.md).
  const readOnlyKey = useMail((s) =>
    s.accounts
      .filter((a) => a.readOnly)
      .map((a) => a.id)
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
      const readOnly = new Set(readOnlyKey === "" ? [] : readOnlyKey.split(",").map(Number));
      const unseen = rows
        .slice(0, MAX_CONVERSATION)
        .filter((r) => !r.flags.seen && !readOnly.has(r.accountId))
        .map((r) => r.id);
      if (unseen.length > 0) void client.message.setFlags({ ids: unseen, changes: { seen: true } }).catch(() => {});
    })().catch((err: unknown) => {
      if (!cancelled) setError(err instanceof Error ? err.message : String(err));
    });
    return () => {
      cancelled = true;
    };
  }, [client, id, trashKey, readOnlyKey]);

  if (error) return <Empty text={error} />;
  if (!items) return null;
  const shown = showAll ? items : items.slice(0, MAX_CONVERSATION);
  return (
    <div className="divide-y divide-separator">
      {shown.map((s) => (
        <ConversationMessage key={s.id} summary={s} onAddress={onAddress} />
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

function ConversationMessage({
  summary,
  onAddress,
}: {
  summary: MessageSummary;
  onAddress: MessageHeaderProps["onAddress"];
}) {
  const client = useClient();
  const vips = useMail((s) => s.vips);
  const vipAddresses = useMemo(() => vipSet(vips), [vips]);
  const [message, setMessage] = useState<Message | null>(null);
  const [rendering, setRendering] = useState<Rendering | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadingRemote, setLoadingRemote] = useState(false);
  const [currentSummary, setCurrentSummary] = useState(summary);
  const [reminderSheet, setReminderSheet] = useState<Date | null>(null);
  const [more, setMore] = useState<{ x: number; y: number } | null>(null);
  const [headers, setHeaders] = useState<string | null>(null);
  const [source, setSource] = useState<MessageSource | null>(null);
  const readOnly = useMail((s) => s.accounts.find((account) => account.id === summary.accountId)?.readOnly === true);
  const id = summary.id;
  const part = message ? invitationPart(message.parts) : undefined;
  const { invitation, busy, onAnswer } = useInvitation(id, part !== undefined);
  const unsubscribe = useUnsubscribe(id, !!message?.listUnsubscribe, readOnly);
  const { timeZone, locale, now } = useCalendarFrame();

  useEffect(() => {
    let stopped = false;
    let request = 0;
    const refresh = () => {
      const version = ++request;
      void client.message
        .summaries({ ids: [id] })
        .then((rows) => {
          const row = rows[0];
          if (!stopped && request === version && row) setCurrentSummary(row);
        })
        .catch((err: unknown) => console.warn("refresh message summary", err));
    };
    const off = client.transport.onEvent((event) => {
      if (event.event === "message.changed" && event.data.ids.includes(id)) refresh();
    });
    refresh();
    return () => {
      stopped = true;
      off();
    };
  }, [client, id]);

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

  const moreAction = async (action: string) => {
    if (action === "headers") {
      if (headers !== null) setHeaders(null);
      else setHeaders((await client.message.source({ id })).headers);
    } else if (action === "source") {
      setSource(await client.message.source({ id }));
    } else if (action === "save" && isTauri()) {
      const subject = (summary.subject.trim() || "(no subject)").replace(/[/\\]/g, "-");
      const path = await save({
        title: "Save Message",
        defaultPath: `${subject}.eml`,
        filters: [{ name: "Email Message", extensions: ["eml"] }],
      });
      if (path !== null) await client.message.save({ id, path });
    } else if (action === "print") {
      window.print();
    }
  };

  return (
    <article aria-label={summary.subject} className="pb-2">
      {message && (
        <MessageHeader
          message={message}
          onAddress={onAddress}
          onMore={setMore}
          vip={isVip(message.summary.from.address, vipAddresses)}
        />
      )}
      {more && (
        <ContextMenu
          {...more}
          items={[
            { kind: "item", id: "headers", label: headers === null ? "Show All Headers" : "Hide All Headers" },
            { kind: "item", id: "source", label: "Raw Source…" },
            { kind: "separator" },
            { kind: "item", id: "save", label: "Save As…" },
            { kind: "item", id: "print", label: "Print…" },
          ]}
          onSelect={(action) => {
            void moreAction(action).catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)));
          }}
          onClose={() => setMore(null)}
        />
      )}
      {headers !== null && (
        <section aria-label="All Headers" className="px-4 pb-4">
          <pre className="whitespace-pre-wrap break-words select-text font-mono text-[11px] leading-4 text-secondary">
            {headers}
          </pre>
        </section>
      )}
      {source && <RawSourceSheet text={source.text} truncated={source.truncated} onClose={() => setSource(null)} />}
      {currentSummary.remindAt && (
        <ReminderBanner
          when={whenText(new Date(currentSummary.remindAt), now, timeZone, locale)}
          disabled={readOnly}
          onChange={() => setReminderSheet(new Date(currentSummary.remindAt ?? ""))}
          onClear={() => {
            void remindMessages(client, [id]).catch((err: unknown) => console.warn("clear reminder", err));
          }}
        />
      )}
      {reminderSheet && (
        <TimeSheet
          title="Remind Me"
          initial={reminderSheet}
          now={now}
          timeZone={timeZone}
          onCancel={() => setReminderSheet(null)}
          onChoose={(at) => {
            if (!readOnly)
              void remindMessages(client, [id], at).catch((err: unknown) => console.warn("change reminder", err));
            setReminderSheet(null);
          }}
        />
      )}
      {unsubscribe.info && (
        <UnsubscribeBanner
          info={unsubscribe.info}
          method={unsubscribe.method}
          busy={unsubscribe.busy}
          failed={unsubscribe.failed}
          onUnsubscribe={unsubscribe.onUnsubscribe}
        />
      )}
      {invitation && (
        <InvitationCard
          invitation={invitation}
          timeZone={timeZone}
          locale={locale}
          busy={busy}
          onAnswer={onAnswer}
          onShowInCalendar={() => {
            const ui = useUI.getState();
            const e = invitation.event;
            ui.selectOccurrence(
              invitation.eventId ? { eventId: invitation.eventId, recurrenceId: "" } : null,
              e.allDay ? e.startDate : zoned(e.start, timeZone).date,
            );
            ui.setModule("calendar");
          }}
        />
      )}
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
      {message && (
        <AttachmentStrip
          parts={invitation ? message.parts.filter((p) => p !== part) : message.parts}
          onOpen={openPart}
        />
      )}
    </article>
  );
}

function useUnsubscribe(id: number, active: boolean, readOnly: boolean) {
  const client = useClient();
  const [info, setInfo] = useState<Unsubscribe | null>(null);
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const running = useRef(false);
  useEffect(() => {
    if (!active) return;
    let cancelled = false;
    void client.message.unsubscribeInfo({ id }).then(
      (result) => {
        if (!cancelled) setInfo(result);
      },
      (err: unknown) => console.warn("load unsubscribe info", err),
    );
    return () => {
      cancelled = true;
    };
  }, [client, id, active]);
  const methods = info?.methods.filter((method) => method !== "mail" || !readOnly) ?? [];
  const onUnsubscribe = () => {
    if (running.current || methods.length === 0) return;
    running.current = true;
    setBusy(true);
    setFailed(false);
    void (async () => {
      for (const method of methods) {
        let result: UnsubscribeResult;
        try {
          result = await client.message.unsubscribe({ id, method });
        } catch (err: unknown) {
          console.warn("unsubscribe", err);
          continue;
        }
        if (result.url) openLink(result.url);
        try {
          setInfo(await client.message.unsubscribeInfo({ id }));
        } catch (err: unknown) {
          console.warn("refresh unsubscribe info", err);
        }
        return;
      }
      setFailed(true);
    })().finally(() => {
      running.current = false;
      setBusy(false);
    });
  };
  return { info: active ? info : null, method: methods[0], busy, failed, onUnsubscribe };
}

function useInvitation(messageId: number, active: boolean) {
  const client = useClient();
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [busy, setBusy] = useState(false);
  const refresh = useRef<() => void>(() => {});
  useEffect(() => {
    if (!active) return;
    let stopped = false;
    let request = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const load = () => {
      const version = ++request;
      void client.calendar.invitation({ messageId }).then(
        (inv) => {
          if (!stopped && request === version) setInvitation(inv);
        },
        () => {
          if (!stopped && request === version) setInvitation(null);
        },
      );
    };
    const soon = () => {
      clearTimeout(timer);
      timer = setTimeout(load, 100);
    };
    refresh.current = soon;
    load();
    const off = client.transport.onEvent((event) => {
      if (event.event === "calendar.changed") soon();
    });
    return () => {
      stopped = true;
      clearTimeout(timer);
      off();
      refresh.current = () => {};
    };
  }, [client, messageId, active]);
  // A ref, not busy: two clicks before the card re-renders (a double
  // click) must still send one answer.
  const answering = useRef(false);
  const onAnswer = (answer: Answer) => {
    if (answering.current) return;
    answering.current = true;
    setBusy(true);
    void client.calendar
      .respond({ messageId, answer })
      .catch((err: unknown) => console.warn("answer invitation", err))
      .finally(() => {
        answering.current = false;
        setBusy(false);
        refresh.current();
      });
  };
  return { invitation: active ? invitation : null, busy, onAnswer };
}
