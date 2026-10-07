// useView: a ViewModel for a query, re-rendering the caller on every change.
import { useEffect, useState, useSyncExternalStore } from "react";

import type { ViewQuery } from "../rpc/gen/api";
import { useClient } from "./session";
import { ViewModel } from "./view";

const noSubscribe = () => () => {};
const zero = () => 0;

/**
 * useView opens a view for query (closing the previous one when the query
 * changes) and returns its ViewModel, or null before it exists.
 */
export function useView(query: ViewQuery): ViewModel | null {
  const client = useClient();
  const key = JSON.stringify(query);
  const [model, setModel] = useState<ViewModel | null>(null);
  useEffect(() => {
    const m = new ViewModel(client, JSON.parse(key) as ViewQuery);
    setModel(m);
    return () => m.close();
  }, [client, key]);
  useSyncExternalStore(model?.subscribe ?? noSubscribe, model?.getVersion ?? zero);
  return model;
}
