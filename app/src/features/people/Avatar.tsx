// A person's avatar: their photo, else initials on a tone (docs/specs/pim-ui.md).
import { avatarTone, initials } from "../../lib/format";

// Whole class names let Tailwind generate every tone and supported size.
const TONES: Record<number, string> = {
  0: "bg-avatar-0",
  1: "bg-avatar-1",
  2: "bg-avatar-2",
  3: "bg-avatar-3",
  4: "bg-avatar-4",
  5: "bg-avatar-5",
  6: "bg-avatar-6",
  7: "bg-avatar-7",
};
const SIZES = { 28: "size-7", 48: "size-12", 64: "size-16" };

/** AvatarProps are an avatar's inputs. */
export interface AvatarProps {
  name: string;
  email: string;
  /** Diameter in pixels: 28, 48 or 64. */
  size: 28 | 48 | 64;
  /** A data: URL of the photo, when there is one. */
  photo?: string;
}

/** Avatar shows a photo, or initials on the email's tone. */
export function Avatar({ name, email, size, photo }: AvatarProps) {
  const shape = `${SIZES[size]} shrink-0 rounded-full`;
  if (photo) return <img src={photo} alt="" className={`${shape} object-cover`} />;
  return (
    <div
      aria-hidden="true"
      className={`${shape} flex items-center justify-center font-semibold text-accent-contrast ${size === 64 ? "text-[20px]" : "text-[13px]"} ${TONES[avatarTone(email)] ?? ""}`}
    >
      {initials({ name, address: email })}
    </div>
  );
}
