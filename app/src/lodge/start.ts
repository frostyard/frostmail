// Starting inside lodge's shell (docs/specs/lodge-app.md, The web build):
// connect to the shell, open the app at the frame's route, keep the route
// and theme in step, and reach maild through lodge's /app/mail/api.
import type { Connect } from "../data/session";
import { connectLodgeAPI } from "../rpc/transport/lodge";
import { type Lodge, setLodge } from "./lodge";
import { wireShell } from "./shell";

/** frameRoute is the route of a frame at pathname, under the build's base (/app/mail/). */
export function frameRoute(pathname: string, base: string): string {
  const root = base.replace(/\/+$/, "");
  if (pathname !== root && !pathname.startsWith(`${root}/`)) return "/";
  return pathname.slice(root.length) || "/";
}

/**
 * startInLodge connects to the shell with connect (lodge-client's
 * connectLodge) and returns how Session reaches maild.
 */
export async function startInLodge(connect: () => Promise<Lodge>, loc: Location = window.location): Promise<Connect> {
  const l = await connect();
  setLodge(l);
  wireShell(l, frameRoute(loc.pathname, import.meta.env.BASE_URL) + loc.search + loc.hash);
  return () => connectLodgeAPI(l.apiURL());
}
