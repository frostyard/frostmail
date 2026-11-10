// Query helpers shared by the window's integration tests.

/**
 * senderName matches a sender button, whose accessible name starts with the
 * person's visible name and ends with the address (reader/MessageHeader.tsx).
 */
export const senderName = (address: string) => (name: string) => name.endsWith(address);
