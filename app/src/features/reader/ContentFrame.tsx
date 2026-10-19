// The reader's page from lodge's content node (ADR-0041;
// docs/specs/lodge-app.md, The web build). maild keeps the sanitized page,
// and lodge serves it from another origin under a sandbox without scripts,
// so this frame cannot reach into it: it fills the reader below the
// message header and scrolls on its own. Links target _blank, and the
// sandbox lets them open a tab, never a script or a form.

/** ContentFrameProps are the frame's inputs. */
export interface ContentFrameProps {
  /** The page's URL on lodge's content origin. */
  src: string;
}

/** ContentFrame shows a reader page from lodge-content. */
export function ContentFrame({ src }: ContentFrameProps) {
  return (
    <iframe
      title="Message"
      sandbox="allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
      src={src}
      className="block w-full border-0 bg-white"
      style={{ height: "var(--reader-height, 80vh)" }}
    />
  );
}
