// A text-only message body (docs/specs/ui.md, Reader: Body): quoted lines are
// indented bars instead of ">" prefixes, long quotes start collapsed, links are
// clickable, and the signature is dimmed.

import { type ReactNode, useMemo, useState } from "react";

/** TextBlock is a run of lines at one quote level, or the signature. */
export type TextBlock = { kind: "text"; quote: number; lines: string[] } | { kind: "signature"; lines: string[] };

type TextRun = Extract<TextBlock, { kind: "text" }>;

const LINK_PATTERN = /https?:\/\/[^\s<>"]+|www\.[^\s<>"]+|[\w.+-]+@[\w-]+(?:\.[\w-]+)+/;
const TRAILING_PUNCTUATION = ".,;:!?)";

function splitQuote(line: string): { level: number; content: string } {
  let i = 0;
  let level = 0;
  while (i < line.length) {
    const ch = line[i];
    if (ch === ">") {
      level += 1;
      i += 1;
      continue;
    }
    if (level > 0 && ch === " " && line[i + 1] === ">") {
      i += 1;
      continue;
    }
    break;
  }
  if (level > 0 && line[i] === " ") {
    i += 1;
  }
  return { level, content: line.slice(i) };
}

/** parsePlainText splits a body into quote-level blocks and the signature. */
export function parsePlainText(text: string): TextBlock[] {
  const raw = text.replace(/\r\n/g, "\n").split("\n");
  let start = 0;
  let end = raw.length;
  while (start < end && (raw[start] ?? "").trim() === "") start += 1;
  while (end > start && (raw[end - 1] ?? "").trim() === "") end -= 1;

  let sigStart = -1;
  for (let i = start; i < end; i += 1) {
    if ((raw[i] ?? "") === "-- ") {
      sigStart = i;
      break;
    }
  }

  const blocks: TextBlock[] = [];
  let current: TextRun | null = null;
  const body = sigStart < 0 ? raw.slice(start, end) : raw.slice(start, sigStart);
  for (const line of body) {
    const { level, content } = splitQuote(line);
    if (current !== null && current.quote === level) {
      current.lines.push(content);
      continue;
    }
    current = { kind: "text", quote: level, lines: [content] };
    blocks.push(current);
  }
  if (sigStart >= 0) {
    blocks.push({ kind: "signature", lines: raw.slice(sigStart + 1, end) });
  }
  return blocks;
}

function hrefFor(match: string): string {
  if (match.startsWith("www.")) return `https://${match}`;
  if (/^https?:\/\//.test(match)) return match;
  return `mailto:${match}`;
}

function linkNodes(line: string, key: string, onOpenLink: (url: string) => void): ReactNode[] {
  const re = new RegExp(LINK_PATTERN.source, "g");
  const nodes: ReactNode[] = [];
  let last = 0;
  let n = 0;
  let match = re.exec(line);
  while (match !== null) {
    const full = match[0] ?? "";
    let cut = full.length;
    while (cut > 0 && TRAILING_PUNCTUATION.includes(full[cut - 1] ?? "")) cut -= 1;
    if (cut > 0) {
      const text = full.slice(0, cut);
      if (match.index > last) nodes.push(line.slice(last, match.index));
      const href = hrefFor(text);
      nodes.push(
        <a
          key={`${key}-${n}`}
          href={href}
          className="text-accent"
          onClick={(event) => {
            event.preventDefault();
            onOpenLink(href);
          }}
        >
          {text}
        </a>,
      );
      last = match.index + cut;
      n += 1;
    }
    match = re.exec(line);
  }
  if (last < line.length) nodes.push(line.slice(last));
  return nodes;
}

function lineNodes(lines: string[], key: string, onOpenLink: (url: string) => void): ReactNode[] {
  const nodes: ReactNode[] = [];
  lines.forEach((line, i) => {
    if (i > 0) nodes.push("\n");
    nodes.push(...linkNodes(line, `${key}-${i}`, onOpenLink));
  });
  return nodes;
}

// Whole class names, so Tailwind generates them.
const QUOTE_COLORS = ["border-quote-1", "border-quote-2", "border-quote-3"];

function quoteClass(level: number): string {
  if (level < 1) return "";
  return `border-l-2 pl-2 ${QUOTE_COLORS[(level - 1) % 3]}`;
}

const COLLAPSE_AFTER = 4;
const COLLAPSED_LINES = 2;

/** PlainTextProps are the body's inputs. */
export interface PlainTextProps {
  text: string;
  /** A link was clicked: an http(s) URL or a mailto: URL. */
  onOpenLink: (url: string) => void;
}

interface BlockViewProps {
  block: TextBlock;
  open: boolean;
  onToggle: () => void;
  onOpenLink: (url: string) => void;
}

function BlockView(props: BlockViewProps) {
  const { block, open, onToggle, onOpenLink } = props;
  if (block.kind === "signature") {
    return (
      <div data-signature className="text-secondary">
        {lineNodes(block.lines, "sig", onOpenLink)}
      </div>
    );
  }
  const collapsible = block.quote > 0 && block.lines.length > COLLAPSE_AFTER;
  const shown = collapsible && !open ? block.lines.slice(0, COLLAPSED_LINES) : block.lines;
  const children = lineNodes(shown, `b${block.quote}`, onOpenLink);
  if (collapsible) {
    children.push("\n");
    children.push(
      <button key="toggle" type="button" className="text-[12px] text-accent" aria-expanded={open} onClick={onToggle}>
        {open ? "See Less" : "See More"}
      </button>,
    );
  }
  return (
    <div
      data-quote-level={String(block.quote)}
      className={quoteClass(block.quote)}
      style={block.quote > 0 ? { marginLeft: `${(block.quote - 1) * 10}px` } : undefined}
    >
      {children}
    </div>
  );
}

function blockKey(block: TextBlock): string {
  if (block.kind === "signature") return "signature";
  return `text-${block.quote}-${block.lines.join("\n")}`;
}

/** PlainText renders a text body with quotes, links and the signature. */
export function PlainText(props: PlainTextProps) {
  const blocks = useMemo(() => parsePlainText(props.text), [props.text]);
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());
  const items = useMemo(() => {
    const seen = new Map<string, number>();
    return blocks.map((block, index) => {
      const base = blockKey(block);
      const count = seen.get(base) ?? 0;
      seen.set(base, count + 1);
      return { block, index, key: count === 0 ? base : `${base}#${count}` };
    });
  }, [blocks]);

  const toggle = (index: number) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });

  return (
    <div className="select-text whitespace-pre-wrap break-words text-reader-body p-5">
      {items.map((item) => (
        <BlockView
          key={item.key}
          block={item.block}
          open={expanded.has(item.index)}
          onToggle={() => toggle(item.index)}
          onOpenLink={props.onOpenLink}
        />
      ))}
    </div>
  );
}
