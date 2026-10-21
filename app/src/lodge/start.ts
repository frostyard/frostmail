// Starting inside lodge's shell (docs/specs/lodge-app.md, The web build):
// connect to the shell, open the app at the frame's route, keep the route
// and theme in step, and reach maild through lodge's /app/mail/api.
import type { Connect } from "../data/session";
import { connectLodgeAPI } from "../rpc/transport/lodge";
import { type Lodge, setLodge } from "./lodge";
import { wireShell } from "./shell";

/**
 * startInLodge connects to the shell with connect (lodge-client's
 * connectLodge) and returns how Session reaches maild.
 */
export async function startInLodge(connect: () => Promise<Lodge>): Promise<Connect> {
  const l = await connect();
  setLodge(l);
  // lodge-client replays nothing: the frame's route is history.location.
  const loc = l.history.location;
  wireShell(l, loc.pathname + loc.search + loc.hash);
  return () => connectLodgeAPI(l);
}
