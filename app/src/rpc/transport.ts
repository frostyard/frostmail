// The line-level contract between the generated Client (gen/api.ts) and a
// connection to maild: the rpc subpath of @frostyard/lodge-client
// (client/v0.3.0), whose JsonRpcSession multiplexes JSON-RPC in the
// webview (docs/specs/rpc-protocol.md). The session hands events over in
// the kit's envelope; the app types what they carry (gen/api.ts), and
// asTransport is the one place the two meet.
import type { Transport as RpcTransport } from "@frostyard/lodge-client/rpc";

import type { Event } from "./gen/api";

export type { EventEnvelope } from "@frostyard/lodge-client/rpc";
export { JsonRpcSession, RPCError } from "@frostyard/lodge-client/rpc";

/** Transport is what the app's client calls, with maild's events typed. */
export interface Transport {
  call<T>(method: string, params: unknown): Promise<T>;
  /** Subscribe to server events; returns an unsubscribe function. */
  onEvent(handler: (event: Event) => void): () => void;
  /** Called once when the connection ends. */
  onClose(handler: (reason: string) => void): () => void;
  /** Where a mail part loads from, when the transport serves one. */
  blobURL?(hash: string, ext: string): string;
}

/** asTransport is the subpath's session with maild's events (gen/api.ts). */
export function asTransport(session: RpcTransport): Transport {
  const transport: Transport = {
    call: <T>(method: string, params: unknown) => session.call<T>(method, params),
    onEvent: (handler) => session.onEvent((envelope) => handler(envelope as Event)),
    onClose: (handler) => session.onClose(handler),
  };
  const blobURL = session.blobURL;
  if (blobURL !== undefined) transport.blobURL = blobURL;
  return transport;
}
