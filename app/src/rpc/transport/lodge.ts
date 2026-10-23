// The lodge transport: maild's API over lodge's /app/mail/api WebSocket,
// opened and kept by @frostyard/lodge-client/rpc's connectLodgeAPI
// (client/v0.3.2): it opens while the page is visible, pings every 30 s so
// lodge keeps the session, and reconnects after a drop
// (docs/specs/lodge-app.md, The web build).
import { connectLodgeAPI as connect } from "@frostyard/lodge-client/rpc";
import type { Lodge } from "../../lodge/lodge";
import { asTransport, type Transport } from "../transport";

/** connectLodgeAPI is maild's API through lodge's shell. */
export function connectLodgeAPI(lodge: Lodge): Promise<Transport> {
  return connect(lodge).then((api) => {
    // Session owns reconnecting, with a fresh connection and its hello and
    // subscribe; stop this one reconnecting too, so two sockets never live.
    const transport = asTransport(api);
    transport.onClose(() => api.close());
    return transport;
  });
}
