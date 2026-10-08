// The compose window's TipTap editor (docs/specs/compose-ui.md, Body and
// Formatting): StarterKit (paragraphs, lists, quotes, bold, italic,
// underline, strike, links) and images limited to what the app can show
// without the network.
import Image from "@tiptap/extension-image";
import type { Editor, Extensions } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";

import type { FormatCommand, FormatKey } from "../features/compose/ComposeToolbar";

/** LINK_PROTOCOLS are the only link targets the editor keeps. */
const LINK_URL = /^(https?:\/\/|mailto:)/i;

/** isAllowedLink reports whether a URL may be a link in a message. */
export function isAllowedLink(url: string): boolean {
  return LINK_URL.test(url.trim());
}

/**
 * SafeImage keeps only quoted inline images (mailpart:) and data: images.
 * Any other image, such as one pasted from a web page, is dropped when the
 * HTML is parsed, so the compose window never fetches remote content
 * (ADR-0005); the app's CSP would block it anyway.
 */
export const SafeImage = Image.extend({
  parseHTML() {
    return [
      {
        tag: "img[src]",
        getAttrs: (el: HTMLElement) => (/^(mailpart:|data:image\/)/i.test(el.getAttribute("src") ?? "") ? null : false),
      },
    ];
  },
});

/** composeExtensions is the editor's schema. */
export function composeExtensions(): Extensions {
  return [
    StarterKit.configure({
      heading: false,
      code: false,
      codeBlock: false,
      horizontalRule: false,
      link: {
        openOnClick: false,
        autolink: true,
        protocols: ["mailto"],
        isAllowedUri: (url) => isAllowedLink(url),
      },
    }),
    SafeImage.configure({ inline: true }),
  ];
}

const KEYS: FormatKey[] = ["bold", "italic", "underline", "strike", "bulletList", "orderedList", "blockquote", "link"];

/** activeFormats reports which marks and nodes are active at the selection. */
export function activeFormats(editor: Editor): Partial<Record<FormatKey, boolean>> {
  const out: Partial<Record<FormatKey, boolean>> = {};
  for (const k of KEYS) out[k] = editor.isActive(k);
  return out;
}

/** runFormat applies a format bar command; link is handled by the caller (it asks for a URL). */
export function runFormat(editor: Editor, cmd: Exclude<FormatCommand, "link">): void {
  const c = editor.chain().focus();
  switch (cmd) {
    case "bold":
      c.toggleBold().run();
      break;
    case "italic":
      c.toggleItalic().run();
      break;
    case "underline":
      c.toggleUnderline().run();
      break;
    case "strike":
      c.toggleStrike().run();
      break;
    case "bulletList":
      c.toggleBulletList().run();
      break;
    case "orderedList":
      c.toggleOrderedList().run();
      break;
    case "blockquote":
      c.toggleBlockquote().run();
      break;
    case "clear":
      c.unsetAllMarks().clearNodes().run();
      break;
  }
}

/** setLink links the selection to url, or removes the link when url is empty; it returns false for a URL that is not allowed. */
export function setLink(editor: Editor, url: string): boolean {
  const href = url.trim();
  const c = editor.chain().focus().extendMarkRange("link");
  if (href === "") {
    c.unsetLink().run();
    return true;
  }
  if (!isAllowedLink(href)) return false;
  c.setLink({ href }).run();
  return true;
}

/** isBlank reports whether the editor holds no text and no images. */
export function isBlank(editor: Editor): boolean {
  let images = false;
  editor.state.doc.descendants((node) => {
    if (node.type.name === "image") images = true;
    return !images;
  });
  return !images && editor.state.doc.textContent.trim() === "";
}
