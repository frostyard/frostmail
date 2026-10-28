// The production transport: lines travel over the Tauri bridge
// (src-tauri/src/bridge.rs) to maild's Unix socket, or to lodge's
// /app/mail/api for a maild behind lodge (docs/design/desktop.md, On a
// maild behind lodge), which closes a session that sends nothing for 90 s.
import { Channel, invoke } from "@tauri-apps/api/core";

import { asTransport, JsonRpcSession, type Transport } from "../transport";

type BridgeMessage = { kind: "line"; line: string } | { kind: "closed"; error: string };

/** PING_MS is how often the app pings a maild behind lodge. */
export const PING_MS = 30_000;

let remote = false;
let flatpak = false;

/** remoteMaild reports whether the last connection reached maild through lodge. */
export function remoteMaild(): boolean {
  return remote;
}

/** inFlatpak reports whether the app runs in its Flatpak (ADR-0046, Files in
 * the sandbox): Save As then asks the file chooser portal for a path. */
export function inFlatpak(): boolean {
  return flatpak;
}

/** filesAsBytes reports whether attaching sends a file's bytes (draft.attachData)
 * rather than its path, because maild cannot read a path the window sees: on a
 * maild behind lodge, and in the Flatpak (docs/specs/compose-ui.md, Attachments). */
export function filesAsBytes(): boolean {
  return remote || flatpak;
}

export async function connectTauri(): Promise<Transport> {
  const session = new JsonRpcSession((line) => invoke("maild_send", { line }));
  const channel = new Channel<BridgeMessage>();
  let timer: ReturnType<typeof setInterval> | undefined;
  channel.onmessage = (m) => {
    if (m.kind === "line") {
      session.receive(m.line);
      return;
    }
    clearInterval(timer);
    session.close(m.error);
  };
  const connected = await invoke<{ remote: boolean } | null>("maild_connect", { onMessage: channel });
  remote = connected?.remote === true;
  // The Flatpak's maild cannot read the paths the window sees outside its
  // sandbox, so it takes bytes and asks the portal (ADR-0046). A shell without
  // the command is no Flatpak.
  flatpak = (await invoke<boolean>("in_flatpak").catch(() => false)) === true;
  if (remote) timer = setInterval(() => void session.call("rpc.ping", {}).catch(() => {}), PING_MS);
  return asTransport(session);
}
