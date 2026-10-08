// The attachment strip under the compose window's editor: a chip per stored
// attachment with its size and a Remove button, a chip per file still being
// attached, and the total against the size limit
// (docs/specs/compose-ui.md, Layout: Attachments).

import { File, FileArchive, FileImage, FileText, LoaderCircle, X } from "lucide-react";
import type { ReactNode } from "react";

import { formatSize } from "../../lib/format";
import type { DraftAttachment } from "../../rpc/gen/api";

/** PendingAttachment is a file still being attached. */
export interface PendingAttachment {
  /** Unique among the pending files; the React key. */
  key: string;
  filename: string;
}

/** ComposeAttachmentsProps are the attachment strip's inputs. */
export interface ComposeAttachmentsProps {
  attachments: DraftAttachment[];
  pending: PendingAttachment[];
  /** The size limit in bytes; the total turns red above it. */
  limit: number;
  onRemove: (id: number) => void;
}

/** CHIP is the shared shape of an attachment and a pending chip. */
const CHIP = "flex h-8 items-center gap-1.5 rounded-md border border-separator bg-window px-2";

/** NAME is the file name span: 13/18, truncated at 200 wide. */
const NAME = "max-w-[200px] truncate text-[13px] leading-[18px]";

/** ARCHIVE_TYPES are the content types shown with the archive icon. */
const ARCHIVE_TYPES: readonly string[] = [
  "application/zip",
  "application/gzip",
  "application/x-gzip",
  "application/x-tar",
  "application/x-7z-compressed",
];

/** fileIcon is the chip's 16px icon for a content type. */
function fileIcon(contentType: string): ReactNode {
  const cls = "shrink-0 text-secondary";
  if (contentType.startsWith("image/")) {
    return <FileImage size={16} className={cls} />;
  }
  if (contentType.startsWith("text/") || contentType === "application/pdf") {
    return <FileText size={16} className={cls} />;
  }
  if (ARCHIVE_TYPES.includes(contentType)) {
    return <FileArchive size={16} className={cls} />;
  }
  return <File size={16} className={cls} />;
}

/** AttachmentChip is one stored attachment: icon, name, size and Remove. */
function AttachmentChip({ attachment, onRemove }: { attachment: DraftAttachment; onRemove: (id: number) => void }) {
  const label = `Remove ${attachment.filename}`;
  return (
    <div data-attachment className={CHIP}>
      {fileIcon(attachment.contentType)}
      <span title={attachment.filename} className={NAME}>
        {attachment.filename}
      </span>
      <span className="text-[12px] leading-4 text-secondary">{formatSize(attachment.size)}</span>
      <button type="button" aria-label={label} title={label} onClick={() => onRemove(attachment.id)}>
        <X size={14} />
      </button>
    </div>
  );
}

/** PendingChip is a file still being attached: a spinner and the name, nothing else. */
function PendingChip({ file }: { file: PendingAttachment }) {
  return (
    <div data-attachment aria-busy="true" className={CHIP}>
      <LoaderCircle size={16} className="shrink-0 animate-spin text-secondary" />
      <span title={file.filename} className={NAME}>
        {file.filename}
      </span>
    </div>
  );
}

/** ComposeAttachments lists a draft's attachments. */
export function ComposeAttachments({ attachments, pending, limit, onRemove }: ComposeAttachmentsProps) {
  if (attachments.length === 0 && pending.length === 0) {
    return null;
  }
  const total = attachments.reduce((sum, attachment) => sum + attachment.size, 0);
  const over = total > limit;
  const count = attachments.length;
  return (
    <section
      aria-label="Attachments"
      className="flex flex-wrap items-center gap-2 border-t border-separator bg-banner px-3 py-2"
    >
      {attachments.map((attachment) => (
        <AttachmentChip key={attachment.id} attachment={attachment} onRemove={onRemove} />
      ))}
      {pending.map((file) => (
        <PendingChip key={file.key} file={file} />
      ))}
      {count > 0 && (
        <span className={`ml-auto text-[12px] leading-4 tabular-nums ${over ? "text-flag-1" : "text-secondary"}`}>
          {`${count} ${count === 1 ? "file" : "files"}, ${formatSize(total)}`}
          {over ? ` — over the ${formatSize(limit)} limit` : ""}
        </span>
      )}
    </section>
  );
}
