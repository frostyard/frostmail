// The reader: the selected message's conversation (docs/design/app.md, Reader).

import { Menu as ContextMenu, type MenuEntry as MenuItem } from "@frostyard/lodge-ui/react";
import { invoke, isTauri } from "@tauri-apps/api/core";
import {
  forwardRef,
  type MouseEvent as ReactMouseEvent,
  type RefObject,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from "react";
import { useClient } from "../data/session";
import { useMail, useUI } from "../data/stores";
import { TimeSheet } from "../features/later/TimeSheet";
import { CollapsedMessage } from "../features/reader/CollapsedMessage";
import { ContentFrame } from "../features/reader/ContentFrame";
import { FindBar } from "../features/reader/FindBar";
import { type Answer, InvitationCard, invitationPart } from "../features/reader/InvitationCard";
import { MessageFrame } from "../features/reader/MessageFrame";
import {
  AttachmentStrip,
  BlockedBanner,
  MessageHeader,
  type MessageHeaderProps,
  ReminderBanner,
  RemoteBanner,
} from "../features/reader/MessageHeader";
import { PlainText } from "../features/reader/PlainText";
import { RawSourceSheet } from "../features/reader/RawSourceSheet";
import { SenderCheckBanner } from "../features/reader/SenderCheckBanner";
import { UnsubscribeBanner } from "../features/reader/UnsubscribeBanner";
import { zoned } from "../lib/calendarDates";
import { whenText } from "../lib/later";
import { isVip, vipSet } from "../lib/vips";
import { IN_LODGE, lodge, openOutside } from "../lodge/lodge";
import { permalink } from "../lodge/routes";
import type {
  Address,
  Client,
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
import { saveMessage } from "./saveMessage";
import { useCalendarFrame } from "./useCalendar";

/** MAX_CONVERSATION is how many messages the reader shows before "Show earlier". */
export const MAX_CONVERSATION = 20;

interface MoreRequest {
  id: number;
  action: "headers" | "source";
  count: number;
}

/** ReaderHandle lets the window page the reader, run More Actions and find
 *  in the message shown. */
export interface ReaderHandle {
  page: (direction: 1 | -1) => void;
  focus: () => void;
  more: (action: "headers" | "source") => void;
  /** find opens the find bar, or focuses its field and selects its text when
   *  it is open; false when no message shows, or in lodge's web build, whose
   *  browser finds in the reader's frame itself (ADR-0052). */
  find: () => boolean;
}

/**
 * openLink opens a URL in the system browser; in lodge, through the shell,
 * which opens a canonical lodge URL (another app's, or mail's own
 * permanent route) in place and anything else in a new tab.
 */
export function openLink(url: string): void {
  if (isTauri()) void invoke("open_link", { url }).catch((err: unknown) => console.warn("open link", err));
  else if (!openOutside(url)) window.open(url, "_blank", "noopener,noreferrer");
}

/**
 * reportContext tells lodge's companion which message is shown, by its
 * permanent route when it has a Message-ID, so a link made from it lasts
 * (docs/specs/lodge-app.md, The web build).
 */
async function reportContext(client: Client, id: number, title: string, stale: () => boolean): Promise<void> {
  const l = lodge();
  if (!l) return;
  const m = await client.message.get({ id }).catch(() => null);
  if (stale()) return;
  const route = m?.messageId ? permalink(m.messageId) : `/message/${id}`;
  l.context({ kind: "message", ref: `mail${route}`, title });
}

/** useReaderHeight sets --reader-height on el to its height: in lodge the
 * message frame fills the reader (ContentFrame). */
function useReaderHeight(el: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    const node = el.current;
    if (!IN_LODGE || !node) return;
    const set = () => node.style.setProperty("--reader-height", `${node.clientHeight}px`);
    set();
    const observer = new ResizeObserver(set);
    observer.observe(node);
    return () => observer.disconnect();
  }, [el]);
}

/** ReaderContainer shows the selection in the reader pane. */
export const ReaderContainer = forwardRef<ReaderHandle>(function ReaderContainer(_props, ref) {
  const { selected, setFocus } = useUI();
  const module = useUI((state) => state.module);
  const [contact, setContact] = useState<{ address: Address; at: { x: number; y: number } } | null>(null);
  const openContact = useCallback((address: Address, at: { x: number; y: number }) => setContact({ address, at }), []);
  const closeContact = useCallback(() => setContact(null), []);
  const scroller = useRef<HTMLElement>(null);
  // The find bar lives outside the scroller, so it stays in view. It belongs
  // to the selection and the module it opened on, so it closes when the
  // reader shows another selection or another module (docs/specs/ui.md, Find
  // in the message).
  const [finding, setFinding] = useState(false);
  const findInput = useRef<HTMLInputElement>(null);
  useReaderHeight(scroller);
  const [moreRequest, setMoreRequest] = useState<MoreRequest | null>(null);
  const id = selected.length === 1 ? selected[0] : undefined;
  const findKey = `${module}:${id ?? ""}`;
  const [findShownFor, setFindShownFor] = useState(findKey);
  if (findShownFor !== findKey) {
    setFindShownFor(findKey);
    setFinding(false);
  }
  useEffect(() => {
    // lodge's companion learns what is on screen; Conversation names a message.
    if (id === undefined) lodge()?.context({ kind: null, ref: "", title: "" });
  }, [id]);
  useEffect(() => {
    if (!finding) return;
    findInput.current?.focus();
    findInput.current?.select();
  }, [finding]);
  const closeFind = useCallback(() => {
    setFinding(false);
    // The reader has the keys again once the bar goes.
    scroller.current?.focus();
  }, []);
  useImperativeHandle(ref, () => ({
    page: (direction) => {
      const el = scroller.current;
      if (el) el.scrollBy({ top: direction * el.clientHeight * 0.9 });
    },
    focus: () => scroller.current?.focus(),
    more: (action) => {
      if (id !== undefined) setMoreRequest((previous) => ({ id, action, count: (previous?.count ?? 0) + 1 }));
    },
    find: () => {
      if (IN_LODGE || id === undefined) return false;
      if (finding) {
        findInput.current?.focus();
        findInput.current?.select();
        return true;
      }
      setFinding(true);
      return true;
    },
  }));
  return (
    <div className="flex h-full flex-col">
      {finding && id !== undefined && <FindBar scroller={scroller.current} onClose={closeFind} inputRef={findInput} />}
      <section
        ref={scroller}
        // biome-ignore lint/a11y/noNoninteractiveTabindex: the reader is a focusable, scrollable pane.
        tabIndex={0}
        aria-label="Message"
        data-print-area
        onFocus={() => setFocus("reader")}
        className="min-h-0 flex-1 overflow-y-auto bg-window outline-none"
      >
        {selected.length === 0 && <Empty text="No Message Selected" />}
        {selected.length > 1 && <Empty text={`${selected.length} Messages Selected`} />}
        {id !== undefined && <Conversation key={id} id={id} onAddress={openContact} moreRequest={moreRequest} />}
        {contact && <ContactCardContainer key={contact.address.address} {...contact} onClose={closeContact} />}
      </section>
    </div>
  );
});

function Empty({ text }: { text: string }) {
  return <div className="flex h-full items-center justify-center text-empty text-secondary">{text}</div>;
}

function Conversation({
  id,
  onAddress,
  moreRequest,
}: {
  id: number;
  onAddress: MessageHeaderProps["onAddress"];
  moreRequest: MoreRequest | null;
}) {
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
  const { now } = useCalendarFrame();
  // The selected message and what was unread when it opened show expanded;
  // the rest collapse to a row each (docs/specs/ui.md, Reader, P-310).
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(() => new Set([id]));
  const setOpen = useCallback((message: number, open: boolean) => {
    setExpanded((previous) => {
      if (previous.has(message) === open) return previous;
      const next = new Set(previous);
      if (open) next.add(message);
      else next.delete(message);
      return next;
    });
  }, []);
  // A More Actions request for the collapsed selected message expands it,
  // and the expanded message then replays that request (moreFrom).
  const lastMoreRequest = useRef(moreRequest?.count ?? 0);
  // A message's entry is used once: the message takes it when it runs the
  // request (onMoreTaken), so a later mount does not replay it.
  const moreFrom = useRef(new Map<number, number>());
  const opened = useRef(false);
  useEffect(() => {
    if (!moreRequest || moreRequest.count <= lastMoreRequest.current) return;
    lastMoreRequest.current = moreRequest.count;
    if (moreRequest.id !== id || expanded.has(id)) return;
    moreFrom.current.set(id, moreRequest.count - 1);
    setOpen(id, true);
  }, [moreRequest, id, expanded, setOpen]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const [s] = await client.message.summaries({ ids: [id] });
      if (!s || cancelled) return;
      void reportContext(client, id, s.subject, () => cancelled);
      let rows = [s];
      if (s.threadId !== 0) rows = await client.thread.messages({ id: s.threadId });
      const trash = new Set(trashKey === "" ? [] : trashKey.split(",").map(Number));
      const inTrash = (r: MessageSummary) => r.mailboxIds.some((mb) => trash.has(mb));
      rows = rows.filter((r) => r.id === id || !inTrash(r) || inTrash(s)).reverse();
      if (cancelled) return;
      setItems(rows);
      // Only the first load decides what opens expanded: a reload (Trash or
      // a read-only account changed) keeps what the user expanded or collapsed.
      if (!opened.current) {
        opened.current = true;
        const unread = rows.filter((r) => !r.flags.seen).map((r) => r.id);
        if (unread.length > 0) setExpanded((previous) => new Set([...previous, ...unread]));
      }
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
  // A conversation of 1 message shows it expanded, with no collapse.
  const collapsible = items.length > 1;
  return (
    <div className="divide-y divide-separator">
      {shown.map((s) =>
        !collapsible || expanded.has(s.id) ? (
          <ConversationMessage
            key={s.id}
            summary={s}
            onAddress={onAddress}
            moreRequest={moreRequest}
            moreFrom={moreFrom.current.get(s.id)}
            onMoreTaken={() => moreFrom.current.delete(s.id)}
            onCollapse={
              collapsible
                ? () => {
                    moreFrom.current.delete(s.id);
                    setOpen(s.id, false);
                  }
                : undefined
            }
          />
        ) : (
          <CollapsedMessage key={s.id} summary={s} now={now} onExpand={() => setOpen(s.id, true)} />
        ),
      )}
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
  moreRequest,
  moreFrom,
  onMoreTaken,
  onCollapse,
}: {
  summary: MessageSummary;
  onAddress: MessageHeaderProps["onAddress"];
  moreRequest: MoreRequest | null;
  /** The More Actions count after which this message replays requests; the current count when absent. */
  moreFrom?: number | undefined;
  /** Says the message took the requests up to moreRequest's count. */
  onMoreTaken?: (() => void) | undefined;
  /** Collapses the message to its row, in a conversation of 2 or more. */
  onCollapse?: (() => void) | undefined;
}) {
  const client = useClient();
  const vips = useMail((s) => s.vips);
  const vipAddresses = useMemo(() => vipSet(vips), [vips]);
  const [message, setMessage] = useState<Message | null>(null);
  const [rendering, setRendering] = useState<Rendering | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loadingRemote, setLoadingRemote] = useState(false);
  const [currentSummary, setCurrentSummary] = useState(summary);
  const senderAddress = currentSummary.from.address;
  const blocked = useMail((s) => s.blocked.some((sender) => sender.address === senderAddress.toLowerCase()));
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
  const shell = lodge();

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
    client.message.render({ id, content: IN_LODGE || undefined }).then((r) => !cancelled && setRendering(r), fail);
    return () => {
      cancelled = true;
    };
  }, [client, id]);

  const loadRemote = useCallback(() => {
    setLoadingRemote(true);
    client.message
      .render({ id, remote: true, content: IN_LODGE || undefined })
      .then(setRendering, () => {})
      .finally(() => setLoadingRemote(false));
  }, [client, id]);

  const openPart = useCallback(
    (part: Part) => {
      void client.message.part({ id, path: part.path }).then((f) => {
        const l = lodge();
        if (isTauri())
          void invoke("open_part", { path: f.path, contentPath: f.contentPath ?? null, filename: f.filename }).catch(
            (err: unknown) => console.warn("open part", err),
          );
        else if (l && f.contentPath) l.open(l.contentURL(f.contentPath));
        else window.open(`/mailpart/${f.path}`, "_blank", "noopener,noreferrer");
      });
    },
    [client, id],
  );

  // In lodge, a part saves as a download from the content origin
  // (docs/specs/lodge-app.md, The web build).
  const savePart = useCallback(
    (part: Part) => {
      void client.message
        .part({ id, path: part.path })
        .then((f) => {
          const l = lodge();
          if (l && f.contentPath) l.open(`${l.contentURL(f.contentPath)}?download=1`);
        })
        .catch((err: unknown) => console.warn("save part", err));
    },
    [client, id],
  );

  const moreAction = useCallback(
    async (action: string) => {
      if (action === "headers") {
        if (headers !== null) setHeaders(null);
        else setHeaders((await client.message.source({ id })).headers);
      } else if (action === "source") {
        setSource(await client.message.source({ id }));
      } else if (action === "save") {
        await saveMessage(client, id, summary.subject);
      } else if (action === "print") {
        window.print();
      }
    },
    [client, id, headers, summary.subject],
  );

  // New articles start at the current count so retained requests cannot
  // replay after a selection change or revealing earlier messages.
  const lastMoreRequest = useRef(moreFrom ?? moreRequest?.count ?? 0);
  useEffect(() => {
    if (!moreRequest || moreRequest.count <= lastMoreRequest.current) return;
    lastMoreRequest.current = moreRequest.count;
    onMoreTaken?.();
    if (moreRequest.id === id)
      void moreAction(moreRequest.action).catch((err: unknown) =>
        setError(err instanceof Error ? err.message : String(err)),
      );
  }, [moreRequest, id, moreAction, onMoreTaken]);

  return (
    <article aria-label={summary.subject} className="pb-2">
      {message && (
        // biome-ignore lint/a11y/noStaticElementInteractions lint/a11y/useKeyWithClickEvents: a click on the header collapses the message, as in Mail.app; the header's controls keep their keys, and Tab moves past the message (docs/specs/ui.md, Reader).
        <div onClick={onCollapse ? (event) => collapseFrom(event, onCollapse) : undefined}>
          <MessageHeader
            message={message}
            onAddress={onAddress}
            onMore={setMore}
            vip={isVip(message.summary.from.address, vipAddresses)}
          />
        </div>
      )}
      {message?.senderCheck === "fail" && (
        <SenderCheckBanner
          address={message.summary.from.address}
          showing={headers !== null}
          onToggle={() => {
            void moreAction("headers").catch((err: unknown) =>
              setError(err instanceof Error ? err.message : String(err)),
            );
          }}
        />
      )}
      {more && (
        <ContextMenu
          {...more}
          items={[
            {
              kind: "item",
              id: "headers",
              label: headers === null ? "Show All Headers" : "Hide All Headers",
              keys: "Ctrl+Shift+H",
            },
            { kind: "item", id: "source", label: "Raw Source…", keys: "Ctrl+Alt+U" },
            { kind: "separator" },
            { kind: "item", id: "save", label: "Save As…" },
            { kind: "item", id: "print", label: "Print…", keys: "Ctrl+P" },
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
      {source && (
        <RawSourceSheet
          text={source.text}
          truncated={source.truncated}
          onClose={() => setSource(null)}
          onSave={() => {
            void saveMessage(client, id, summary.subject).catch((err: unknown) =>
              setError(err instanceof Error ? err.message : String(err)),
            );
          }}
        />
      )}
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
      {blocked && (
        <BlockedBanner
          onUnblock={() => {
            void client.block
              .remove({ addresses: [senderAddress] })
              .catch((err: unknown) => console.warn("unblock sender", err));
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
          shell ? (
            // In lodge, HTML shows only from lodge-content: a page too
            // large for maild to keep (contentPath absent) is never put in
            // a frame on lodge's own origin (docs/specs/lodge-app.md).
            rendering.contentPath ? (
              <ContentFrame src={shell.contentURL(rendering.contentPath)} />
            ) : (
              <p role="status" className="px-5 py-4 text-reader-meta text-secondary">
                This message is too large to show here. Save As… in More Actions downloads it.
              </p>
            )
          ) : (
            <MessageFrame html={rendering.html} onOpenLink={openLink} quoteSource={summary.id} />
          )
        ) : (
          <div data-quote-source={summary.id}>
            <PlainText text={rendering.text} onOpenLink={openLink} />
          </div>
        ))}
      {message && (
        <AttachmentStrip
          parts={invitation ? message.parts.filter((p) => p !== part) : message.parts}
          onOpen={openPart}
          {...(IN_LODGE ? { onSave: savePart } : {})}
        />
      )}
    </article>
  );
}

/**
 * collapseFrom collapses a message on a click in its header, except a click
 * on a control in it (an address, More Actions) or one that ends a text
 * selection (docs/specs/ui.md, Reader).
 */
function collapseFrom(event: ReactMouseEvent<HTMLElement>, collapse: () => void): void {
  const target = event.target;
  if (target instanceof Element && target.closest("button, a, input, select, textarea, [role='button']")) return;
  const selection = window.getSelection();
  if (selection && !selection.isCollapsed && event.currentTarget.contains(selection.anchorNode)) return;
  collapse();
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
