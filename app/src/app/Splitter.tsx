// A vertical pane divider with a wider invisible drag handle
// (docs/specs/ui.md, Layout: Panes).
import { useRef } from "react";

/** Splitter reports horizontal drags, in pixels, from where they started. */
export function Splitter(props: { label: string; onResize: (deltaFromStart: number) => void; onEnd?: () => void }) {
  const start = useRef<number | null>(null);
  return (
    // biome-ignore lint/a11y/useSemanticElements: a pointer-driven pane divider, not a horizontal rule.
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={props.label}
      className="relative z-10 w-px shrink-0 cursor-col-resize bg-separator"
      onPointerDown={(e) => {
        start.current = e.clientX;
        e.currentTarget.setPointerCapture(e.pointerId);
      }}
      onPointerMove={(e) => {
        if (start.current !== null) props.onResize(e.clientX - start.current);
      }}
      onPointerUp={(e) => {
        start.current = null;
        e.currentTarget.releasePointerCapture(e.pointerId);
        props.onEnd?.();
      }}
    >
      <div className="absolute inset-y-0 -left-[3px] w-[6px]" />
    </div>
  );
}
