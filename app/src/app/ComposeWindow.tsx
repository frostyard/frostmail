// A compose window: one draft, edited in place and saved to maild as it
// changes (docs/specs/compose-ui.md). maild keeps the draft and its server
// copy; this window only edits, attaches and sends.
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWebview } from "@tauri-apps/api/webview";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { ask, open as openFiles } from "@tauri-apps/plugin-dialog";
import { EditorContent, useEditor, useEditorState } from "@tiptap/react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import { useClient } from "../data/session";
import { ComposeAttachments, type PendingAttachment } from "../features/compose/ComposeAttachments";
import { ComposeHeader } from "../features/compose/ComposeHeader";
import { type ComposeCommand, ComposeToolbar, FormatBar, type FormatCommand } from "../features/compose/ComposeToolbar";
import { isValidAddress } from "../lib/addressParse";
import type { Client, Draft, DraftAttachment, DraftContent, Identity } from "../rpc/gen/api";
import { activeFormats, composeExtensions, isBlank, runFormat, setLink } from "./composeEditor";

/** SAVE_MS is how long edits wait before draft.update. */
export const SAVE_MS = 500;

/** ATTACHMENT_LIMIT is the attachment size the strip warns about (maild's limit). */
const ATTACHMENT_LIMIT = 25_000_000;

/** ComposeWindow loads a draft and edits it. */
export function ComposeWindow(props: { draftId: number; fresh: boolean }) {
  const client = useClient();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [failure, setFailure] = useState("");

  useEffect(() => {
    let live = true;
    client.draft
      .get({ id: props.draftId })
      .then((d) => live && setDraft(d))
      .catch((err: unknown) => live && setFailure(message(err)));
    return () => {
      live = false;
    };
  }, [client, props.draftId]);

  if (!draft) {
    return (
      <div
        data-tauri-drag-region
        className="flex h-full items-center justify-center bg-window text-empty text-secondary"
      >
        {failure === "" ? "Opening draft…" : `This draft cannot be opened: ${failure}`}
      </div>
    );
  }
  return <Composer client={client} initial={draft} fresh={props.fresh} />;
}

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function closeWindow(): void {
  if (isTauri()) void getCurrentWindow().close();
  else window.close();
}

function basename(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

/** Composer edits one loaded draft. */
function Composer(props: { client: Client; initial: Draft; fresh: boolean }) {
  const { client, initial } = props;
  const id = initial.id;
  const [content, setContent] = useState<DraftContent>(initial.content);
  const [attachments, setAttachments] = useState<DraftAttachment[]>(initial.attachments);
  const [pending, setPending] = useState<PendingAttachment[]>([]);
  const [identities, setIdentities] = useState<Identity[]>([]);
  const [showBcc, setShowBcc] = useState(initial.content.bcc.length > 0);
  const [formatBar, setFormatBar] = useState(false);
  const [maximized, setMaximized] = useState(false);
  const [error, setError] = useState("");
  const [linkUrl, setLinkUrl] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // The content as last edited, the save timer, and whether maild is behind.
  const latest = useRef(content);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const dirty = useRef(false);
  const touched = useRef(false);
  const saving = useRef<Promise<void>>(Promise.resolve());

  const flush = useCallback((): Promise<void> => {
    clearTimeout(timer.current);
    if (!dirty.current) return saving.current;
    dirty.current = false;
    const next = saving.current.then(() =>
      client.draft.update({ id, content: latest.current }).then(
        () => undefined,
        (err: unknown) => {
          dirty.current = true;
          setError(`The draft was not saved: ${message(err)}`);
        },
      ),
    );
    saving.current = next;
    return next;
  }, [client, id]);

  const change = useCallback(
    (patch: Partial<DraftContent>) => {
      touched.current = true;
      dirty.current = true;
      latest.current = { ...latest.current, ...patch };
      setContent(latest.current);
      clearTimeout(timer.current);
      timer.current = setTimeout(() => void flush(), SAVE_MS);
    },
    [flush],
  );

  // Save what is left when the window goes away.
  useEffect(() => {
    const onHide = () => void flush();
    window.addEventListener("pagehide", onHide);
    return () => {
      window.removeEventListener("pagehide", onHide);
      void flush();
    };
  }, [flush]);

  const editor = useEditor({
    extensions: composeExtensions(),
    content: initial.content.html,
    autofocus: initial.content.to.length > 0 ? "start" : false,
    editorProps: {
      attributes: { "aria-label": "Message body", class: "compose-body" },
    },
    onUpdate: ({ editor: e }) => change({ html: e.getHTML() }),
  });
  const active = useEditorState({ editor, selector: ({ editor: e }) => (e ? activeFormats(e) : {}) }) ?? {};

  useEffect(() => {
    void client.identity
      .list({ accountId: initial.accountId })
      .then(setIdentities)
      .catch(() => {});
  }, [client, initial.accountId]);

  useEffect(() => {
    const title = content.subject.trim() === "" ? "New Message" : content.subject;
    if (isTauri()) void getCurrentWindow().setTitle(title);
    else document.title = title;
  }, [content.subject]);

  useEffect(() => {
    if (!isTauri()) return;
    const win = getCurrentWindow();
    const update = () => void win.isMaximized().then(setMaximized);
    update();
    const off = win.onResized(update);
    return () => void off.then((f) => f());
  }, []);

  const recipients = [...content.to, ...content.cc, ...content.bcc];
  const canSend =
    !busy && pending.length === 0 && recipients.length > 0 && recipients.every((a) => isValidAddress(a.address));

  const attachPaths = useCallback(
    (paths: string[]) => {
      for (const path of paths) {
        const key = `${Date.now()}-${Math.random()}`;
        touched.current = true;
        setPending((p) => [...p, { key, filename: basename(path) }]);
        client.draft
          .attach({ id, path })
          .then((a) => setAttachments((list) => [...list, a]))
          .catch((err: unknown) => setError(`${basename(path)} was not attached: ${message(err)}`))
          .finally(() => setPending((p) => p.filter((x) => x.key !== key)));
      }
    },
    [client, id],
  );

  // Files dropped anywhere on the window are attached.
  useEffect(() => {
    if (!isTauri()) return;
    const off = getCurrentWebview().onDragDropEvent((e) => {
      if (e.payload.type === "drop") attachPaths(e.payload.paths);
    });
    return () => void off.then((f) => f());
  }, [attachPaths]);

  const pickFiles = useCallback(async () => {
    if (!isTauri()) {
      setError("Attaching files needs the Frostmail app.");
      return;
    }
    const picked = await openFiles({ multiple: true, directory: false, title: "Attach Files" });
    if (picked === null) return;
    attachPaths(Array.isArray(picked) ? picked : [picked]);
  }, [attachPaths]);

  const send = useCallback(async () => {
    if (!canSend) return;
    setBusy(true);
    setError("");
    try {
      await flush();
      await client.draft.send({ id });
      closeWindow();
    } catch (err) {
      setError(message(err));
      setBusy(false);
    }
  }, [canSend, flush, client, id]);

  const isEmpty = useCallback(
    () =>
      recipients.length === 0 &&
      content.subject.trim() === "" &&
      attachments.length === 0 &&
      (editor ? isBlank(editor) : true),
    [recipients.length, content.subject, attachments.length, editor],
  );

  const discard = useCallback(async () => {
    if (!isEmpty()) {
      const sure = isTauri()
        ? await ask("This draft will be deleted.", { title: "Delete Draft?", kind: "warning", okLabel: "Delete" })
        : window.confirm("Delete this draft?");
      if (!sure) return;
    }
    clearTimeout(timer.current);
    dirty.current = false;
    try {
      await client.draft.delete({ id });
      closeWindow();
    } catch (err) {
      setError(message(err));
    }
  }, [isEmpty, client, id]);

  const close = useCallback(async () => {
    if (props.fresh && !touched.current) {
      clearTimeout(timer.current);
      await client.draft.delete({ id }).catch(() => {});
    } else {
      await flush();
    }
    closeWindow();
  }, [props.fresh, client, id, flush]);

  const onCommand = useCallback(
    (cmd: ComposeCommand) => {
      switch (cmd) {
        case "send":
          void send();
          break;
        case "attach":
          void pickFiles();
          break;
        case "toggleFormatBar":
          setFormatBar((v) => !v);
          break;
        case "delete":
          void discard();
          break;
        case "minimize":
          if (isTauri()) void getCurrentWindow().minimize();
          break;
        case "toggleMaximize":
          if (isTauri()) void getCurrentWindow().toggleMaximize();
          break;
        case "close":
          void close();
          break;
      }
    },
    [send, pickFiles, discard, close],
  );

  const onFormat = useCallback(
    (cmd: FormatCommand) => {
      if (!editor) return;
      if (cmd === "link") setLinkUrl((editor.getAttributes("link").href as string | undefined) ?? "");
      else runFormat(editor, cmd);
    },
    [editor],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.ctrlKey || e.altKey || e.metaKey) return;
      const k = e.key.toLowerCase();
      const run = (f: () => void) => {
        e.preventDefault();
        f();
      };
      if (k === "enter" && !e.shiftKey) run(() => void send());
      else if (k === "a" && e.shiftKey) run(() => void pickFiles());
      else if (k === "b" && e.shiftKey) run(() => setShowBcc((v) => !v));
      else if (k === "backspace" && !e.shiftKey) run(() => void discard());
      else if (k === "w" && !e.shiftKey) run(() => void close());
      else if (k === "k" && !e.shiftKey) run(() => onFormat("link"));
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [send, pickFiles, discard, close, onFormat]);

  const suggest = useMemo(() => (prefix: string) => client.address.suggest({ prefix, limit: 8 }), [client]);

  return (
    <div className="flex h-full flex-col bg-window text-primary">
      <ComposeToolbar
        subject={content.subject}
        canSend={canSend}
        formatBarShown={formatBar}
        maximized={maximized}
        onCommand={onCommand}
      />
      <ComposeHeader
        content={content}
        identities={identities}
        showBcc={showBcc}
        onChange={change}
        suggest={suggest}
        autoFocusTo={initial.content.to.length === 0}
      />
      {formatBar && <FormatBar active={active} onCommand={onFormat} />}
      {linkUrl !== null && editor && (
        <LinkBar
          initial={linkUrl}
          onDone={(url) => {
            if (url !== null && !setLink(editor, url)) {
              setError("Links must start with http://, https:// or mailto:");
              return;
            }
            setLinkUrl(null);
            editor.commands.focus();
          }}
        />
      )}
      {error !== "" && (
        <div
          role="alert"
          className="flex items-center gap-2 border-b border-separator bg-banner px-4 py-1.5 text-[12px] leading-4 text-flag-1"
        >
          <span className="min-w-0 flex-1">{error}</span>
          <button type="button" onClick={() => setError("")} className="text-accent">
            Dismiss
          </button>
        </div>
      )}
      <div className="min-h-0 flex-1 overflow-y-auto">
        <EditorContent editor={editor} className="h-full" />
      </div>
      <ComposeAttachments
        attachments={attachments}
        pending={pending}
        limit={ATTACHMENT_LIMIT}
        onRemove={(attachmentId) => {
          touched.current = true;
          client.draft
            .detach({ id, attachmentId })
            .then(() => setAttachments((list) => list.filter((a) => a.id !== attachmentId)))
            .catch((err: unknown) => setError(message(err)));
        }}
      />
    </div>
  );
}

/** LinkBar asks for a link's URL above the editor; null cancels. */
function LinkBar(props: { initial: string; onDone: (url: string | null) => void }) {
  const [url, setUrl] = useState(props.initial);
  return (
    <div className="flex h-8 items-center gap-2 border-b border-separator bg-window px-4">
      <input
        type="text"
        aria-label="Link URL"
        placeholder="https://"
        // biome-ignore lint/a11y/noAutofocus: the bar opens to take a URL
        autoFocus
        value={url}
        onChange={(e) => setUrl(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            props.onDone(url);
          } else if (e.key === "Escape") {
            e.preventDefault();
            props.onDone(null);
          }
        }}
        className="min-w-0 flex-1 border-none bg-transparent text-[13px] leading-[18px] text-primary outline-none"
      />
      <button type="button" onClick={() => props.onDone(url)} className="text-[12px] leading-4 text-accent">
        {url.trim() === "" ? "Remove Link" : "Apply"}
      </button>
    </div>
  );
}
