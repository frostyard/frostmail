// Save As of a whole message (docs/specs/ui.md, More Actions), for the
// reader's menu and Raw Source's sheet.
import { invoke, isTauri } from "@tauri-apps/api/core";
import { save } from "@tauri-apps/plugin-dialog";

import { lodge } from "../lodge/lodge";
import type { Client } from "../rpc/gen/api";
import { inFlatpak, remoteMaild } from "../rpc/transport/tauri";

/**
 * saveMessage is Save As, the message as the server holds it in an .eml
 * file (docs/specs/lodge-app.md, The web build): in lodge the browser
 * downloads it from the content route; the desktop app asks where, then
 * maild writes it there, or on a maild behind lodge the app downloads it
 * from lodge's content node there. In the Flatpak the file chooser portal's
 * dialog asks where, since the sandbox can write nowhere else
 * (ADR-0046, Files in the sandbox); elsewhere it is Tauri's dialog.
 */
export async function saveMessage(client: Client, id: number, subject: string): Promise<void> {
  const contentPath = async () => {
    const s = await client.message.source({ id, content: true });
    if (!s.contentPath) throw new Error("maild did not name the message on lodge's content node");
    return s.contentPath;
  };
  const l = lodge();
  if (l) {
    l.open(`${l.contentURL(await contentPath())}?download=1`);
    return;
  }
  if (!isTauri()) return;
  const name = (subject.trim() || "(no subject)").replace(/[/\\]/g, "-");
  let path: string | null;
  if (inFlatpak()) {
    path = await invoke<string | null>("portal_save_file", {
      title: "Save Message",
      currentName: `${name}.eml`,
      filterName: "Email Message",
      extension: "eml",
    });
  } else {
    path = await save({
      title: "Save Message",
      defaultPath: `${name}.eml`,
      filters: [{ name: "Email Message", extensions: ["eml"] }],
    });
  }
  if (path === null) return;
  if (remoteMaild()) await invoke("save_content", { contentPath: await contentPath(), path });
  else await client.message.save({ id, path });
}
