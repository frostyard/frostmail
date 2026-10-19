// Opening a message by its permanent route, /message/<Message-ID>
// (docs/specs/lodge-app.md, The web build): the shell's route asks, and
// maild resolves the Message-ID to the message a reader wants.
import { create } from "zustand";
import type { Client } from "../rpc/gen/api";
import { RPCError } from "../rpc/transport";
import { revealMessage } from "./openMessage";

/** usePermalink holds the Message-ID a permanent route asks for, until it resolves. */
export const usePermalink = create<{ messageId: string | null }>(() => ({ messageId: null }));

/** requestPermalink asks to open the message a Message-ID names. */
export function requestPermalink(messageId: string): void {
  usePermalink.setState({ messageId });
}

/** permalinkPending reports whether a permanent route is still resolving. */
export function permalinkPending(): boolean {
  return usePermalink.getState().messageId !== null;
}

/**
 * watchPermalinks resolves each requested Message-ID with
 * message.byMessageId and reveals the message, or leaves the window as it
 * is when none is stored. It returns the cleanup.
 */
export function watchPermalinks(client: Client): () => void {
  const resolve = (messageId: string) => {
    void client.message
      .byMessageId({ messageId })
      .then((s) => {
        if (usePermalink.getState().messageId !== messageId) return;
        return revealMessage(client, s.id);
      })
      .catch((err: unknown) => {
        if (!(err instanceof RPCError)) console.warn("open permalink", err);
      })
      .finally(() => {
        if (usePermalink.getState().messageId === messageId) usePermalink.setState({ messageId: null });
      });
  };
  const pending = usePermalink.getState().messageId;
  if (pending !== null) resolve(pending);
  return usePermalink.subscribe((state, prev) => {
    if (state.messageId !== null && state.messageId !== prev.messageId) resolve(state.messageId);
  });
}
