// Snippets load once and reload after maild's snippet.changed event.
import { useCallback, useEffect, useRef, useState } from "react";

import { useClient } from "../data/session";
import type { Snippet } from "../rpc/gen/api";

/** useSnippets keeps the last successful snippet list. */
export function useSnippets(): Snippet[] {
  const client = useClient();
  const [snippets, setSnippets] = useState<Snippet[]>([]);
  const request = useRef(0);

  const refresh = useCallback(() => {
    const revision = ++request.current;
    void client.snippet
      .list()
      .then((result) => {
        if (revision === request.current) setSnippets(result);
      })
      .catch(() => {});
  }, [client]);

  useEffect(() => {
    refresh();
    const off = client.transport.onEvent((event) => {
      if (event.event === "snippet.changed") refresh();
    });
    return () => {
      request.current++;
      off();
    };
  }, [client, refresh]);

  return snippets;
}
