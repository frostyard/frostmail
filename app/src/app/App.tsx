// The app root: the connection to maild around the main window or, in a
// compose window, the draft it edits (docs/design/app.md).
import { type Connect, Session } from "../data/session";
import { useMail } from "../data/stores";
import { ComposeWindow } from "./ComposeWindow";
import { composeTarget } from "./compose";
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

/** App connects to maild with connect and shows the main window, or a compose window's draft. */
export function App(props: { connect: Connect }) {
  const target = composeTarget();
  return (
    <Session connect={props.connect} fallback={<Connecting />}>
      {target === null ? <MainWindow /> : <ComposeWindow draftId={target.draftId} fresh={target.fresh} />}
    </Session>
  );
}
