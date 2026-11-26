// A message's permanent route in lodge (docs/specs/lodge-app.md, The web
// build): /message/ and its Message-ID as one segment, which maild resolves
// (message.byMessageId). The canonical URL is /mail followed by the route.
// The NSGo client reads the route (cmd/frostmail-nsgo/routes.go); the
// loader names it in the shell's context.

/** permalink is the permanent route of a message by its Message-ID, without angle brackets. */
export function permalink(messageId: string): string {
  return `/message/${encodeURIComponent(messageId)}`;
}
