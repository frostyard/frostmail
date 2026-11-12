// The app root: the connection to maild around the main window, covered by
// the window a route names (docs/design/app.md).
import { type Connect, Session } from "../data/session";
import { useMail } from "../data/stores";
import { useFrameWindow } from "../lodge/frameWindow";
import { ComposeWindow } from "./ComposeWindow";
import { EventWindow } from "./EventWindow";
import { MainWindow } from "./MainWindow";
import { RemindersWindow } from "./RemindersWindow";
import { SettingsWindow } from "./SettingsWindow";

/** Connecting fills the window while it has no maild. */
export function Connecting() {
  const connection = useMail((s) => s.connection);
  const text = connection.state === "lost" ? `Reconnecting to maild… (${connection.reason})` : "Connecting to maild…";
  return (
    <div className="flex h-full items-center justify-center bg-window text-empty text-secondary">
      <div className="flex flex-col items-center">
        <span>{text}</span>
      </div>
    </div>
  );
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

/** App connects to maild with connect and shows the main window, covered by
 * the window a route names. */
export function App(props: { connect: Connect }) {
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      <LodgeFrame />
    </Session>
  );
}
