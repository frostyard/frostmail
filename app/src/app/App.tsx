// The app root: the connection to maild around the main window, a compose
// window's draft, or the settings window (docs/design/app.md).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect, useState } from "react";

import { type Connect, Session } from "../data/session";
import { useMail } from "../data/stores";
import { ComposeWindow } from "./ComposeWindow";
import { composeTarget } from "./compose";
import { MainWindow } from "./MainWindow";
import { SettingsWindow } from "./SettingsWindow";
import { isSettingsWindow } from "./settings";

function Connecting() {
  const connection = useMail((s) => s.connection);
  const text = connection.state === "lost" ? `Reconnecting to maild… (${connection.reason})` : "Connecting to maild…";
  return (
    <div data-tauri-drag-region className="flex h-full items-center justify-center bg-window text-empty text-secondary">
      {text}
    </div>
  );
}

/** useHash re-renders on hash changes (a browser may open #/compose/<id> or #/settings in the same tab). */
function useHash() {
  const [hash, setHash] = useState(window.location.hash);
  useEffect(() => {
    const update = () => setHash(window.location.hash);
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  return hash;
}

/** App connects to maild with connect and shows the main window, a compose window's draft, or the settings. */
export function App(props: { connect: Connect }) {
  useHash();
  const target = composeTarget();
  // Tauri creates windows hidden; show this one now that it has rendered,
  // so it never flashes WebKit's blank white page (src-tauri main.rs).
  useEffect(() => {
    if (isTauri()) void getCurrentWindow().show();
  }, []);
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      {target !== null ? (
        <ComposeWindow draftId={target.draftId} fresh={target.fresh} />
      ) : isSettingsWindow() ? (
        <SettingsWindow />
      ) : (
        <MainWindow />
      )}
    </Session>
  );
}
