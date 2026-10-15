// The sandboxed HTML body (ADR-0005; docs/design/app.md, Reader). The frame
// never gets allow-scripts. allow-same-origin lets this component size the
// frame; maild has already sanitized the HTML and the app CSP forbids the
// network. Links target _blank and allow-popups lets them ask for a new
// window, which the app shell opens in the system browser without creating
// one (ADR-0036): WebKit runs no event listener, not even the parent's,
// inside a frame sandboxed without scripts.
import { useEffect, useMemo, useRef, useState } from "react";

/** MessageFrameProps are the frame's inputs. */
export interface MessageFrameProps {
  /** Sanitized HTML from message.render. */
  html: string;
  /** A link was clicked: an http, https or mailto URL. */
  onOpenLink: (url: string) => void;
}

const OPENABLE = new Set(["http:", "https:", "mailto:"]);

// The page around the message: white paper in both color schemes, as in
// docs/specs/ui.md (Reader: Body). Colors here belong to the message page,
// not the app, so they are literal.
const PAGE_STYLE = [
  "html{color-scheme:light}",
  'body{margin:0;padding:20px;background:#fff;color:#000;font:14px/1.45 "Inter Variable",system-ui,sans-serif;overflow-wrap:anywhere}',
  "img{max-width:100%;height:auto}",
].join("");

/** frameDocument wraps sanitized HTML in the frame's page. */
export function frameDocument(html: string, dev = import.meta.env.DEV): string {
  // WebKitGTK does not load mailpart: into the dev server's page; Vite
  // serves the same files under /mailpart/ (vite.config.ts).
  const body = dev ? html.replaceAll("mailpart://localhost/", "/mailpart/") : html;
  return `<!doctype html><html><head><meta charset="utf-8"><meta name="color-scheme" content="light"><base target="_blank"><style>${PAGE_STYLE}</style></head><body>${body}</body></html>`;
}

/** linkTarget returns the URL a click should open, or null. */
export function linkTarget(target: EventTarget | null): string | null {
  if (!target || !("closest" in target) || typeof target.closest !== "function") return null;
  const a = (target as Element).closest("a[href]");
  const href = a?.getAttribute("href");
  if (!href) return null;
  try {
    const url = new URL(href);
    return OPENABLE.has(url.protocol) ? url.href : null;
  } catch {
    return null;
  }
}

/** MessageFrame shows sanitized HTML in a sandboxed, content-sized frame. */
export function MessageFrame({ html, onOpenLink }: MessageFrameProps) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [height, setHeight] = useState(0);
  const doc = useMemo(() => frameDocument(html), [html]);
  const openLink = useRef(onOpenLink);
  openLink.current = onOpenLink;

  useEffect(() => {
    const el = frame.current;
    if (!el) return;
    let observer: ResizeObserver | undefined;
    let page: Document | null = null;
    // Browsers (the dev gateway) run these listeners; WebKitGTK does not, and
    // its links reach the shell as new-window requests instead.
    const onClick = (e: MouseEvent) => {
      // Cancel every navigation; only links open, outside the app.
      e.preventDefault();
      e.stopPropagation();
      const url = linkTarget(e.target);
      if (url) openLink.current(url);
    };
    const onSubmit = (e: Event) => e.preventDefault();
    const onLoad = () => {
      page = el.contentDocument;
      if (!page) return;
      const root = page.documentElement;
      const measure = () => setHeight(root.scrollHeight);
      measure();
      observer?.disconnect();
      observer = new ResizeObserver(measure);
      observer.observe(root);
      if (page.body) observer.observe(page.body);
      page.addEventListener("click", onClick, true);
      page.addEventListener("auxclick", onClick, true);
      page.addEventListener("submit", onSubmit, true);
    };
    el.addEventListener("load", onLoad);
    // The message may have loaded before this effect ran; it would stay 0px
    // tall waiting for a load event that has already fired.
    const loaded = el.contentDocument;
    if (loaded?.URL === "about:srcdoc" && loaded.readyState === "complete") onLoad();
    return () => {
      el.removeEventListener("load", onLoad);
      observer?.disconnect();
      page?.removeEventListener("click", onClick, true);
      page?.removeEventListener("auxclick", onClick, true);
      page?.removeEventListener("submit", onSubmit, true);
    };
  }, []);

  return (
    <iframe
      ref={frame}
      title="Message"
      sandbox="allow-same-origin allow-popups"
      referrerPolicy="no-referrer"
      srcDoc={doc}
      className="block w-full border-0"
      style={{ height }}
    />
  );
}
