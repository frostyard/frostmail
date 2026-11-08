// The presentational Snippets pane (docs/specs/settings-ui.md, Snippets pane).
import { Minus, Plus } from "lucide-react";
import { type FormEvent, useEffect, useState } from "react";

import { signatureHtml, signatureText } from "../../lib/signature";
import type { Snippet } from "../../rpc/gen/api";
import { ALERT, BUTTON, FIELD, GRID, LABEL, PRIMARY_BUTTON } from "./labels";

export interface SnippetChange {
  name: string;
  html: string;
}

export interface SnippetsPaneProps {
  snippets: Snippet[];
  busy: boolean;
  error: string | null;
  onCreate: (change: SnippetChange) => void;
  onSave: (id: number, change: SnippetChange) => void;
  onRemove: (id: number) => void;
}

const ICON_BUTTON =
  "flex h-6 w-6 items-center justify-center rounded text-secondary hover:bg-selection-inactive disabled:opacity-40";

/** SnippetsPane lists snippets and edits the selected one or a new one. */
export function SnippetsPane({ snippets, busy, error, onCreate, onSave, onRemove }: SnippetsPaneProps) {
  const [selectedId, setSelectedId] = useState<number | null>(snippets[0]?.id ?? null);
  const [adding, setAdding] = useState(false);
  // The name just added, with the snippets listed then: only a snippet that
  // appears after it, not one already there under that name, closes the form.
  const [pending, setPending] = useState<{ name: string; ids: number[] } | null>(null);
  const selected = snippets.find((snippet) => snippet.id === selectedId) ?? snippets[0] ?? null;

  useEffect(() => {
    if (pending !== null) {
      const added = snippets.find(
        (snippet) => !pending.ids.includes(snippet.id) && snippet.name.toLowerCase() === pending.name.toLowerCase(),
      );
      if (added) {
        setSelectedId(added.id);
        setAdding(false);
        setPending(null);
      }
      return;
    }
    if (selectedId !== null && !snippets.some((snippet) => snippet.id === selectedId)) {
      setSelectedId(snippets[0]?.id ?? null);
    }
  }, [snippets, pending, selectedId]);

  const openAdd = () => {
    setPending(null);
    setAdding(true);
  };

  return (
    <div className="flex h-full narrow:flex-col">
      <div className="flex shrink-0 flex-col narrow:max-h-[35%]">
        <ul
          aria-label="Snippets"
          className="w-[220px] flex-1 shrink-0 overflow-y-auto border-r border-separator bg-sidebar py-2 narrow:w-full narrow:border-r-0"
        >
          {snippets.map((snippet) => (
            <li key={snippet.id}>
              <button
                type="button"
                aria-current={snippet.id === selected?.id ? "true" : undefined}
                className={`flex w-full px-3 py-1.5 text-left ${snippet.id === selected?.id ? "bg-selection-sidebar" : ""}`}
                onClick={() => {
                  setSelectedId(snippet.id);
                  setAdding(false);
                  setPending(null);
                }}
              >
                <span className="truncate text-[13px] leading-[18px]">{snippet.name}</span>
              </button>
            </li>
          ))}
        </ul>
        <div className="flex h-8 w-[220px] shrink-0 items-center gap-1 border-t border-r border-separator bg-sidebar px-2 narrow:w-full narrow:border-r-0 narrow:border-b">
          <button
            type="button"
            aria-label="Add Snippet"
            className={ICON_BUTTON}
            disabled={busy || adding}
            onClick={openAdd}
          >
            <Plus size={14} />
          </button>
          <button
            type="button"
            aria-label="Remove Snippet"
            className={ICON_BUTTON}
            disabled={busy || adding || selected === null}
            onClick={() => selected && onRemove(selected.id)}
          >
            <Minus size={14} />
          </button>
        </div>
      </div>
      {adding ? (
        <NewSnippetForm
          busy={busy}
          error={error}
          onCancel={() => {
            setAdding(false);
            setPending(null);
          }}
          onCreate={(change) => {
            setPending({ name: change.name, ids: snippets.map((snippet) => snippet.id) });
            onCreate(change);
          }}
        />
      ) : selected ? (
        <SnippetForm key={selected.id} snippet={selected} busy={busy} error={error} onSave={onSave} />
      ) : (
        <p className="px-6 py-5 text-[13px] leading-[18px] text-secondary">
          Snippets are text you insert in a message with Insert Snippet in the compose window.
        </p>
      )}
    </div>
  );
}

function NewSnippetForm({
  busy,
  error,
  onCancel,
  onCreate,
}: {
  busy: boolean;
  error: string | null;
  onCancel: () => void;
  onCreate: (change: SnippetChange) => void;
}) {
  const [name, setName] = useState("");
  const [text, setText] = useState("");
  const disabled = busy || name.trim() === "";
  return (
    <form
      className="flex flex-1 flex-col px-6 py-5 narrow:min-h-0 narrow:overflow-y-auto narrow:px-4"
      onSubmit={(event) => {
        event.preventDefault();
        if (!disabled) onCreate({ name: name.trim(), html: signatureHtml(text) });
      }}
    >
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">New Snippet</h2>
      <div className={GRID}>
        <label htmlFor="snippet-name" className={LABEL}>
          Name:
        </label>
        <input
          id="snippet-name"
          type="text"
          maxLength={60}
          className={FIELD}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <label htmlFor="snippet-text" className={`${LABEL} self-start pt-1`}>
          Text:
        </label>
        <textarea
          id="snippet-text"
          rows={8}
          className="min-h-[160px] rounded-md border border-separator bg-window p-2 text-[13px] leading-[18px] focus:outline-none focus:ring-2 focus:ring-focus"
          value={text}
          onChange={(event) => setText(event.target.value)}
        />
      </div>
      {error !== null && error !== "" && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
      <div className="mt-auto flex justify-end gap-2 pt-5">
        <button type="button" className={BUTTON} onClick={onCancel}>
          Cancel
        </button>
        <button type="submit" className={PRIMARY_BUTTON} disabled={disabled}>
          Add
        </button>
      </div>
    </form>
  );
}

function SnippetForm({
  snippet,
  busy,
  error,
  onSave,
}: {
  snippet: Snippet;
  busy: boolean;
  error: string | null;
  onSave: (id: number, change: SnippetChange) => void;
}) {
  const storedText = signatureText(snippet.html);
  const [name, setName] = useState(snippet.name);
  const [text, setText] = useState(storedText);

  useEffect(() => {
    setName(snippet.name);
    setText(signatureText(snippet.html));
  }, [snippet.name, snippet.html]);

  const dirty = name.trim() !== snippet.name || text !== storedText;
  const disabled = busy || name.trim() === "" || !dirty;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!disabled) onSave(snippet.id, { name: name.trim(), html: signatureHtml(text) });
  };

  return (
    <form
      className="flex flex-1 flex-col px-6 py-5 narrow:min-h-0 narrow:overflow-y-auto narrow:px-4"
      onSubmit={submit}
    >
      <h2 className="mb-4 text-[15px] leading-5 font-semibold">{snippet.name}</h2>
      <div className={GRID}>
        <label htmlFor="snippet-name" className={LABEL}>
          Name:
        </label>
        <input
          id="snippet-name"
          type="text"
          maxLength={60}
          className={FIELD}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <label htmlFor="snippet-text" className={`${LABEL} self-start pt-1`}>
          Text:
        </label>
        <textarea
          id="snippet-text"
          rows={8}
          className="min-h-[160px] rounded-md border border-separator bg-window p-2 text-[13px] leading-[18px] focus:outline-none focus:ring-2 focus:ring-focus"
          value={text}
          onChange={(event) => setText(event.target.value)}
        />
      </div>
      {error !== null && error !== "" && (
        <p role="alert" className={ALERT}>
          {error}
        </p>
      )}
      <div className="mt-auto flex justify-end pt-5">
        <button type="submit" className={PRIMARY_BUTTON} disabled={disabled}>
          Save
        </button>
      </div>
    </form>
  );
}
