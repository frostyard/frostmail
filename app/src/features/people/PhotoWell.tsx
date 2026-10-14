import { useRef } from "react";
import { Avatar } from "./Avatar";

/** PhotoWellProps supplies the current avatar and local photo actions. */
export interface PhotoWellProps {
  name: string;
  email: string;
  photo?: string;
  onPick: (file: File) => void;
  onRemove?: () => void;
}

/** PhotoWell offers a file picker and removal of the current photo. */
export function PhotoWell({ name, email, photo, onPick, onRemove }: PhotoWellProps) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <div className="mb-4 flex flex-col items-start gap-1">
      <button type="button" aria-label="Change Photo" onClick={() => input.current?.click()}>
        <Avatar name={name} email={email} photo={photo} size={64} />
      </button>
      <input
        ref={input}
        type="file"
        hidden
        aria-label="Photo file"
        accept="image/jpeg,image/png"
        onChange={(event) => {
          const file = event.currentTarget.files?.[0];
          event.currentTarget.value = "";
          if (file) onPick(file);
        }}
      />
      {photo && onRemove && (
        <button type="button" className="text-[12px] leading-4 text-accent hover:underline" onClick={onRemove}>
          Remove Photo
        </button>
      )}
    </div>
  );
}
