// A person's avatar: their photo, else initials on a tone (docs/specs/pim-ui.md).
// Task T-0063 writes it.

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
export function Avatar(_props: AvatarProps) {
  return null;
}
