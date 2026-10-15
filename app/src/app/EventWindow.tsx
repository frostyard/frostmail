// One event's draft and the requests that save it (docs/specs/pim-ui.md).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { X } from "lucide-react";
import { useCallback, useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";

import { useClient } from "../data/session";
import { type CalendarChoice, EventEditor } from "../features/calendar/EventEditor";
import { useNotify } from "../features/calendar/NotifySheet";
import { ScopeSheet } from "../features/calendar/ScopeSheet";
import { ALERT, BUTTON, PRIMARY_BUTTON } from "../features/settings/labels";
import { changedFields, draftOf, type EventDraft, newDraft, sendsInvitees, validate } from "../lib/eventDraft";
import { withNotify } from "../lib/notify";
import type { CalendarEvent, Client, EventScope } from "../rpc/gen/api";
import type { EditorTarget } from "./eventWindow";
import { announceRestore } from "./pimUndo";

interface LoadedEvent {
  event: CalendarEvent | null;
  initial: EventDraft;
  calendars: CalendarChoice[];
  readOnly: boolean;
}

async function loadEvent(client: Client, target: EditorTarget, appZone: string): Promise<LoadedEvent> {
  const [accounts, collections] = await Promise.all([
    client.account.list(),
    client.account.collections({ kind: "calendar" }),
  ]);
  const writable = collections.filter(
    (calendar) => !calendar.readOnly && accounts.some((a) => a.id === calendar.accountId && !a.readOnly),
  );
  const choice = (calendar: (typeof collections)[number]): CalendarChoice => ({
    id: calendar.id,
    name: calendar.name,
    color: calendar.color,
    invites: calendar.invites,
    account: accounts.find((a) => a.id === calendar.accountId)?.email ?? "",
  });
  if (target.kind === "new") {
    const calendarId = target.calendarId ?? writable.find((c) => c.isDefault)?.id ?? writable[0]?.id;
    if (calendarId === undefined) throw new Error("No writable calendar is available.");
    return {
      event: null,
      initial: newDraft(target, calendarId, appZone),
      calendars: writable.map(choice),
      readOnly: false,
    };
  }
  const event = await client.calendar.event({
    id: target.id,
    ...(target.recurrenceId ? { recurrenceId: target.recurrenceId } : {}),
  });
  const readOnly = event.readOnly || !writable.some((c) => c.id === event.calendarId);
  const initial = draftOf(event, appZone);
  const ownCalendar = collections.filter((c) => c.id === event.calendarId);
  const staysInCalendar = (initial.invitees?.length ?? 0) > 0 && ownCalendar.some((c) => c.invites);
  return {
    event,
    initial,
    calendars: (readOnly || staysInCalendar ? ownCalendar : writable).map(choice),
    readOnly,
  };
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function closeWindow(): void {
  if (isTauri()) void getCurrentWindow().close();
  else window.close();
}

/** EventWindow loads and edits its target's event. */
export function EventWindow({ target }: { target: EditorTarget }) {
  const client = useClient();
  const appZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const [loaded, setLoaded] = useState<LoadedEvent | null>(null);
  const [failure, setFailure] = useState("");
  useEffect(() => {
    let live = true;
    setLoaded(null);
    setFailure("");
    void loadEvent(client, target, appZone)
      .then((value) => live && setLoaded(value))
      .catch((error: unknown) => live && setFailure(message(error)));
    return () => {
      live = false;
    };
  }, [client, target, appZone]);
  if (!loaded) {
    return (
      <div className="flex h-full flex-col bg-window text-primary">
        <TitleStrip title="New Event" onClose={closeWindow} />
        <div className="p-5">
          {failure ? (
            <p role="alert" className={ALERT}>
              {failure}
            </p>
          ) : (
            "Loading…"
          )}
        </div>
      </div>
    );
  }
  return <LoadedEditor client={client} loaded={loaded} appZone={appZone} />;
}

function TitleStrip({ title, onClose, busy }: { title: string; onClose: () => void; busy?: boolean }) {
  // Keep the window title in sync with the editor before the updated form is visible.
  useLayoutEffect(() => {
    if (isTauri()) void getCurrentWindow().setTitle(title);
    else document.title = title;
  }, [title]);
  return (
    <div data-tauri-drag-region className="flex h-8 shrink-0 items-center gap-2 border-b border-separator px-3">
      <span data-tauri-drag-region className="min-w-0 flex-1 truncate text-[13px] font-semibold">
        {title}
      </span>
      <button
        type="button"
        aria-label="Close"
        disabled={busy}
        onClick={onClose}
        className="text-secondary disabled:opacity-40"
      >
        <X size={16} aria-hidden="true" />
      </button>
    </div>
  );
}

function LoadedEditor({ client, loaded, appZone }: { client: Client; loaded: LoadedEvent; appZone: string }) {
  const { ask, sheet } = useNotify();
  const { event, initial, calendars, readOnly } = loaded;
  const [draft, setDraft] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [closing, setClosing] = useState(false);
  const [scopeAction, setScopeAction] = useState<"change" | "delete" | null>(null);
  const title = draft.summary || "New Event";
  const errors = validate(draft);
  const invalid = Object.keys(errors).length > 0;
  const invited = Boolean(event?.organizer && !event.organizer.isUser);
  const original = event ? initial : null;
  const cannotInvite = sendsInvitees(original, draft) && !calendars.find((c) => c.id === draft.calendarId)?.invites;
  const cannotSave = invalid || cannotInvite;
  // An incomplete date still counts as an edit, but cannot be converted to RPC fields yet.
  const dirty =
    !readOnly && (original ? invalid || Object.keys(changedFields(original, draft)).length > 0 : draft.summary !== "");
  const zones = useMemo(
    () => [...new Set([appZone, ...(event?.timeZone ? [event.timeZone] : []), ...Intl.supportedValuesOf("timeZone")])],
    [appZone, event?.timeZone],
  );
  const close = useCallback(() => {
    if (busy) return;
    if (dirty) setClosing(true);
    else closeWindow();
  }, [busy, dirty]);
  useEffect(() => {
    const onKey = (key: KeyboardEvent) => {
      if (key.key !== "Escape" || key.defaultPrevented) return;
      key.preventDefault();
      if (scopeAction) setScopeAction(null);
      else if (closing) setClosing(false);
      else close();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [close, closing, scopeAction]);

  const perform = async (action: "change" | "delete", scope?: EventScope) => {
    if (busy || readOnly || (action === "change" && cannotSave) || (action === "delete" && invited)) return;
    setBusy(true);
    setFailure("");
    setScopeAction(null);
    try {
      const occurrence =
        event?.recurring && event.recurrenceId && scope ? { recurrenceId: event.recurrenceId, scope } : {};
      if (action === "delete" && !event) return;
      const fields = action === "change" ? changedFields(original, draft) : {};
      const result = await withNotify((notify) => {
        const notification = notify ? { notify } : {};
        if (action === "delete" && event)
          return client.calendar.delete({ id: event.id, ...occurrence, ...notification });
        return event
          ? client.calendar.update({ id: event.id, ...occurrence, fields, ...notification })
          : client.calendar.create({ calendarId: draft.calendarId, fields, ...notification });
      }, ask);
      if (result === null) return;
      await announceRestore({ module: "calendar", restoreId: result.restoreId }).catch((err: unknown) =>
        console.warn("announce event restore", err),
      );
      closeWindow();
    } catch (error) {
      setFailure(message(error));
    } finally {
      setBusy(false);
    }
  };
  const request = (action: "change" | "delete") => {
    if (busy || readOnly || (action === "change" && cannotSave) || (action === "delete" && invited)) return;
    setClosing(false);
    if (action === "change" && event && !dirty) {
      closeWindow();
    } else if (event?.recurring && event.recurrenceId) {
      setScopeAction(action);
    } else {
      void perform(action);
    }
  };

  return (
    <div className="flex h-full flex-col bg-window text-primary">
      <TitleStrip title={title} onClose={close} busy={busy} />
      <form
        className="flex min-h-0 flex-1 flex-col"
        onSubmit={(submit) => {
          submit.preventDefault();
          request("change");
        }}
      >
        <div className="min-h-0 flex-1 overflow-y-auto">
          <fieldset disabled={busy}>
            <EventEditor
              draft={draft}
              onChange={setDraft}
              errors={errors}
              calendars={calendars}
              zones={zones}
              locale={navigator.language}
              readOnly={readOnly}
              invited={invited}
              suggest={(prefix) => client.address.suggest({ prefix, limit: 8 })}
            />
          </fieldset>
          {failure && (
            <p role="alert" className={`${ALERT} mx-5 mb-4`}>
              {failure}
            </p>
          )}
        </div>
        <footer className="flex shrink-0 items-center gap-2 border-t border-separator p-5">
          {event && !readOnly && !invited && (
            <button
              type="button"
              disabled={busy}
              className="text-[13px] leading-4 text-flag-1 disabled:opacity-40"
              onClick={() => request("delete")}
            >
              Delete Event
            </button>
          )}
          <div className="ml-auto flex gap-2">
            {readOnly ? (
              <button type="button" className={BUTTON} onClick={close}>
                Close
              </button>
            ) : (
              <>
                <button type="button" className={BUTTON} disabled={busy} onClick={close}>
                  Cancel
                </button>
                <button type="submit" className={PRIMARY_BUTTON} disabled={busy || cannotSave}>
                  Save
                </button>
              </>
            )}
          </div>
        </footer>
      </form>
      {scopeAction && (
        <ScopeSheet
          action={scopeAction}
          first={event?.first ?? false}
          onChoose={(scope) => void perform(scopeAction, scope)}
          onCancel={() => setScopeAction(null)}
        />
      )}
      {closing && (
        <CloseSheet
          title={title}
          canSave={!busy && !cannotSave}
          onSave={() => request("change")}
          onCancel={() => setClosing(false)}
        />
      )}
      {sheet}
    </div>
  );
}

function CloseSheet({
  title,
  canSave,
  onSave,
  onCancel,
}: {
  title: string;
  canSave: boolean;
  onSave: () => void;
  onCancel: () => void;
}) {
  const titleId = useId();
  const dialog = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const previous = document.activeElement;
    dialog.current?.querySelector<HTMLButtonElement>("button:not(:disabled)")?.focus();
    return () => {
      if (previous instanceof HTMLElement) previous.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-window/60">
      <div
        ref={dialog}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="w-[420px] max-h-[90vh] overflow-y-auto rounded-[10px] border border-separator bg-window p-5 outline-none"
        onKeyDown={(key) => {
          key.stopPropagation();
          if (key.key === "Escape") {
            key.preventDefault();
            onCancel();
          } else if (key.key === "Tab") {
            const controls = Array.from(key.currentTarget.querySelectorAll<HTMLButtonElement>("button:not(:disabled)"));
            const index = controls.indexOf(document.activeElement as HTMLButtonElement);
            key.preventDefault();
            controls[(index + (key.shiftKey ? controls.length - 1 : 1)) % controls.length]?.focus();
          }
        }}
        onKeyUp={(key) => key.stopPropagation()}
      >
        <h2 id={titleId} className="mb-4 text-[13px] font-semibold">
          Save changes to "{title}"?
        </h2>
        <div className="flex flex-col items-end gap-2">
          <button type="button" className={PRIMARY_BUTTON} disabled={!canSave} onClick={onSave}>
            Save
          </button>
          <button type="button" className={BUTTON} onClick={closeWindow}>
            Don't Save
          </button>
          <button type="button" className={BUTTON} onClick={onCancel}>
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}
