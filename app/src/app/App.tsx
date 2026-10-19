// The app root: the connection to maild around the main window, a compose
// window's draft, an event editor, the reminders, or the settings window
// (docs/design/app.md).
import { isTauri } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect, useState } from "react";

import { type Connect, Session } from "../data/session";
import { useMail } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { IN_LODGE } from "../lodge/lodge";
import { ComposeWindow } from "./ComposeWindow";
import { composeTarget } from "./compose";
import { EventWindow } from "./EventWindow";
import { MainWindow } from "./MainWindow";
import { editorTarget } from "./openEvent";
import { RemindersWindow } from "./RemindersWindow";
import { isRemindersWindow } from "./reminders";
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

/** useHash re-renders on hash changes (a browser may open #/compose/<id>, #/event/<id>, #/reminders or #/settings in the same tab). */
function useHash() {
  const [hash, setHash] = useState(window.location.hash);
  useEffect(() => {
    const update = () => setHash(window.location.hash);
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  return hash;
}

/** LodgeFrame is the app in lodge's frame: the main window, covered by the
 * window a route names, which keeps the main window mounted underneath
 * (lodge/frameWindow.ts). */
function LodgeFrame() {
  const open = useFrameWindow((s) => s.open);
  return (
    <>
      <div className="h-full" hidden={open !== null}>
        <MainWindow />
      </div>
      {open?.kind === "compose" && <ComposeWindow key={open.draftId} draftId={open.draftId} fresh={open.fresh} />}
      {open?.kind === "event" && <EventWindow key={JSON.stringify(open.target)} target={open.target} />}
      {open?.kind === "reminders" && <RemindersWindow />}
      {open?.kind === "settings" && <SettingsWindow />}
    </>
  );
}

/** App connects to maild with connect and shows the main window, a compose window's draft, an event editor, reminders, or settings. */
export function App(props: { connect: Connect }) {
  useHash();
  const target = composeTarget();
  const event = editorTarget();
  // Tauri creates windows hidden; show this one now that it has rendered,
  // so it never flashes WebKit's blank white page (src-tauri main.rs).
  useEffect(() => {
    if (isTauri()) void getCurrentWindow().show();
  }, []);
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      {IN_LODGE ? (
        <LodgeFrame />
      ) : target !== null ? (
        <ComposeWindow draftId={target.draftId} fresh={target.fresh} />
      ) : event !== null ? (
        <EventWindow target={event} />
      ) : isRemindersWindow() ? (
        <RemindersWindow />
      ) : isSettingsWindow() ? (
        <SettingsWindow />
      ) : (
        <MainWindow />
      )}
    </Session>
  );
}
