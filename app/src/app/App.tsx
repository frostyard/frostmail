// The app root: the connection to maild around the main window
// (docs/design/app.md).
import { type Connect, Session } from "../data/session";
import { useMail } from "../data/stores";
import { MainWindow } from "./MainWindow";

function Connecting() {
  const connection = useMail((s) => s.connection);
  const text = connection.state === "lost" ? `Reconnecting to maild… (${connection.reason})` : "Connecting to maild…";
  return (
    <div data-tauri-drag-region className="flex h-full items-center justify-center bg-window text-empty text-secondary">
      {text}
    </div>
  );
}

/** App connects to maild with connect and shows the main window. */
export function App(props: { connect: Connect }) {
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      <MainWindow />
    </Session>
  );
}
