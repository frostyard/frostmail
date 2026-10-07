// A text-only message body (docs/specs/ui.md, Reader: Body). Task T-0028
// implements parsePlainText and PlainText; the stubs find no blocks and
// render the text as it is.

/** TextBlock is a run of lines at one quote level, or the signature. */
export type TextBlock = { kind: "text"; quote: number; lines: string[] } | { kind: "signature"; lines: string[] };

/** parsePlainText splits a body into blocks. */
export function parsePlainText(_text: string): TextBlock[] {
  return [];
}

/** PlainTextProps are the body's inputs. */
export interface PlainTextProps {
  text: string;
  /** A link was clicked: an http(s) URL or a mailto: URL. */
  onOpenLink: (url: string) => void;
}

/** PlainText renders a text body with quotes, links and the signature. */
export function PlainText(props: PlainTextProps) {
  return <div>{props.text}</div>;
}
