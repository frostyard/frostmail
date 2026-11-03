// The sidebar's Follow Up section, fed by maild (docs/specs/ui.md, Sidebar):
// conversations waiting on other people. A row opens its message where it
// lives, in Sent, and selects it.
import { useClient } from "../data/session";
import { FollowUpSection } from "../features/followup/FollowUp";
import type { AwaitingThread } from "../rpc/gen/api";
import { revealMessage } from "./openMessage";
import { useFollowUp } from "./useFollowUp";

/** FollowUpContainer connects the Follow Up section to maild. */
export function FollowUpContainer() {
  const client = useClient();
  const awaiting = useFollowUp();
  const onOpen = (thread: AwaitingThread) => {
    void revealMessage(client, thread.message.id).catch((err: unknown) => console.warn("open follow up", err));
  };
  return <FollowUpSection awaiting={awaiting} onOpen={onOpen} />;
}
