// The lodge transport: maild's API over lodge's /app/mail/api WebSocket,
// one JSON-RPC message per text frame (docs/specs/lodge-app.md, /api).
// lodge closes a session after 90 s without a frame from the browser, so
// this sends rpc.ping every 30 s while the page is visible, and opens a
// socket only while it is.
import { JsonRpcSession, type Transport } from "../transport";

/** PING_MS is how often a visible page pings maild through lodge. */
export const PING_MS = 30_000;

/** whenVisible resolves once the page is visible. */
export function whenVisible(doc: Document = document): Promise<void> {
  if (doc.visibilityState === "visible") return Promise.resolve();
  return new Promise((resolve) => {
    const on = () => {
      if (doc.visibilityState !== "visible") return;
      doc.removeEventListener("visibilitychange", on);
      resolve();
    };
    doc.addEventListener("visibilitychange", on);
  });
}

/** connectLodgeAPI opens maild's API at url (lodge.apiURL()) once the page is visible. */
export async function connectLodgeAPI(url: string, doc: Document = document): Promise<Transport> {
  await whenVisible(doc);
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(url);
    const session = new JsonRpcSession(async (line) => ws.send(line));
    let open = false;
    let timer: ReturnType<typeof setInterval> | undefined;
    const ping = () => {
      if (doc.visibilityState === "visible") void session.call("rpc.ping", {}).catch(() => {});
    };
    const onVisibility = () => {
      if (doc.visibilityState === "visible") ping();
    };
    ws.onmessage = (e) => {
      const line = String(e.data).replace(/\n$/, "");
      if (line !== "") session.receive(line);
    };
    ws.onopen = () => {
      open = true;
      timer = setInterval(ping, PING_MS);
      doc.addEventListener("visibilitychange", onVisibility);
      resolve(session);
    };
    ws.onclose = (e) => {
      clearInterval(timer);
      doc.removeEventListener("visibilitychange", onVisibility);
      if (!open) reject(new Error(`cannot reach maild through lodge (${e.code})`));
      session.close(`lodge closed the connection (${e.code})`);
    };
  });
}
