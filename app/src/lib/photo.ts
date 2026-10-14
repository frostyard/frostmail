import type { Photo } from "../rpc/gen/api";

/** PhotoChange sets, removes or keeps the contact's photo. */
export type PhotoChange = Photo | null | undefined;

/** PHOTO_SIZE is the maximum width and height of a contact photo. */
export const PHOTO_SIZE = 512;

/** scalePhoto crops a centered square and encodes it as a scaled JPEG. */
export async function scalePhoto(file: Blob): Promise<Photo> {
  const bitmap = await createImageBitmap(file);
  const canvas = document.createElement("canvas");
  try {
    const side = Math.min(bitmap.width, bitmap.height);
    canvas.width = canvas.height = Math.min(side, PHOTO_SIZE);
    const context = canvas.getContext("2d");
    if (!context) throw new Error("Could not prepare the photo.");
    context.drawImage(
      bitmap,
      (bitmap.width - side) / 2,
      (bitmap.height - side) / 2,
      side,
      side,
      0,
      0,
      canvas.width,
      canvas.height,
    );
  } finally {
    bitmap.close();
  }
  const blob = await new Promise<Blob>((resolve, reject) => {
    canvas.toBlob(
      (result) => (result ? resolve(result) : reject(new Error("Could not encode the photo."))),
      "image/jpeg",
      0.9,
    );
  });
  const bytes = new Uint8Array(await blob.arrayBuffer());
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return { contentType: "image/jpeg", data: btoa(binary) };
}

/** photoURL embeds a photo without fetching remote content. */
export function photoURL(photo: Photo): string {
  return `data:${photo.contentType};base64,${photo.data}`;
}
