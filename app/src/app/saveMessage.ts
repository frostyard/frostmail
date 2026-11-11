// Save As of a whole message (docs/specs/ui.md, More Actions), for the
// reader's menu and Raw Source's sheet.
import { lodge } from "../lodge/lodge";
import type { Client } from "../rpc/gen/api";

/**
 * saveMessage is Save As, the message as the server holds it in an .eml
 * file (docs/specs/lodge-app.md, The web build): in lodge the browser
 * downloads it from the content route. Outside lodge it does nothing.
 */
export async function saveMessage(client: Client, id: number): Promise<void> {
  const l = lodge();
  if (!l) return;
  const s = await client.message.source({ id, content: true });
  if (!s.contentPath) throw new Error("maild did not name the message on lodge's content node");
  l.open(`${l.contentURL(s.contentPath)}?download=1`);
}
