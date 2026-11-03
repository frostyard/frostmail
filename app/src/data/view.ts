// ViewModel: one maild view as the list sees it (docs/design/app.md, Data).
import type { Client, Event, MessageSummary, ViewDelta, ViewQuery } from "../rpc/gen/api";

/** PAGE is how many rows one view.range request asks for. */
export const PAGE = 100;
// Pages kept on either side of the requested window; farther rows are
// dropped and fetched again if they come back into view.
const KEEP_PAGES = 20;

/**
 * ViewModel keeps a view's count and a sparse window of rows. It opens the
 * view on construction, applies view.delta, refreshes rows named by
 * message.changed, and fetches the rows asked for with ensure. Subscribers
 * are told about every change; the version number identifies a state for
 * useSyncExternalStore.
 */
export class ViewModel {
  /** The open error, if view.open failed. */
  error: string | null = null;
  /** True once view.open answered. */
  ready = false;

  private viewId: number | null = null;
  private total = 0;
  private rows = new Map<number, MessageSummary>();
  private loading = new Set<number>();
  private generation = 0;
  private window: [number, number] = [0, 0];
  private version = 0;
  private closed = false;
  private early: ViewDelta[] = [];
  private readonly listeners = new Set<() => void>();
  private readonly unsubscribe: () => void;

  constructor(
    private readonly client: Client,
    readonly query: ViewQuery,
  ) {
    this.unsubscribe = client.transport.onEvent((e) => this.onEvent(e));
    void this.open();
  }

  /** count is the number of rows in the view. */
  get count(): number {
    return this.total;
  }

  /** row returns the row at index i if it is loaded. */
  row(i: number): MessageSummary | undefined {
    return this.rows.get(i);
  }

  /** indexOf returns the index of a loaded row by message ID, or -1. */
  indexOf(id: number): number {
    for (const [i, r] of this.rows) if (r.id === id) return i;
    return -1;
  }

  /** ensure asks for rows [start, end) and remembers them as the visible window. */
  ensure(start: number, end: number): void {
    this.window = [Math.max(0, start), Math.max(0, end)];
    if (this.viewId === null || this.closed) return;
    const last = Math.min(this.window[1], this.total);
    for (let p = Math.floor(this.window[0] / PAGE); p * PAGE < last; p++) {
      if (!this.loading.has(p) && !this.pageLoaded(p)) void this.fetchPage(p);
    }
  }

  /** subscribe registers a change listener; it returns the unsubscribe function. */
  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  /** getVersion identifies the current state. */
  getVersion = (): number => this.version;

  /** close closes the view in maild and stops listening. */
  close(): void {
    if (this.closed) return;
    this.closed = true;
    this.unsubscribe();
    this.listeners.clear();
    if (this.viewId !== null) void this.client.view.close({ id: this.viewId }).catch(() => {});
  }

  private async open(): Promise<void> {
    try {
      const info = await this.client.view.open({ query: this.query });
      if (this.closed) {
        void this.client.view.close({ id: info.id }).catch(() => {});
        return;
      }
      this.viewId = info.id;
      this.total = info.count;
      this.ready = true;
      for (const d of this.early) if (d.id === info.id) this.applyDelta(d);
      this.early = [];
      this.notify();
      this.ensure(...this.window);
    } catch (err) {
      this.error = err instanceof Error ? err.message : String(err);
      this.notify();
    }
  }

  private onEvent(e: Event): void {
    if (this.closed) return;
    if (e.event === "view.delta") {
      // A delta can overtake the view.open reply; keep it until the ID is known.
      if (this.viewId === null) this.early.push(e.data);
      else if (e.data.id === this.viewId) this.applyDelta(e.data);
    } else if (e.event === "message.changed") {
      void this.refresh(e.data.ids);
    }
  }

  private applyDelta(d: ViewDelta): void {
    this.generation++;
    for (const op of d.ops) {
      const next = new Map<number, MessageSummary>();
      for (const [i, row] of this.rows) {
        if (op.op === "insert") next.set(i >= op.at ? i + op.count : i, row);
        else if (i < op.at) next.set(i, row);
        else if (i >= op.at + op.count) next.set(i - op.count, row);
      }
      this.rows = next;
    }
    this.total = d.count;
    // Replies to requests sent before this delta carry stale indices; they
    // are dropped by the generation check, so ask again.
    this.loading.clear();
    this.notify();
    this.ensure(...this.window);
  }

  private async fetchPage(p: number): Promise<void> {
    const id = this.viewId;
    if (id === null) return;
    const gen = this.generation;
    this.loading.add(p);
    try {
      const rows = await this.client.view.range({ id, start: p * PAGE, end: (p + 1) * PAGE });
      if (this.closed || gen !== this.generation) return;
      this.loading.delete(p);
      rows.forEach((r, i) => {
        this.rows.set(p * PAGE + i, r);
      });
      this.evict();
      this.notify();
    } catch {
      if (gen === this.generation) this.loading.delete(p);
    }
  }

  private async refresh(ids: number[]): Promise<void> {
    const wanted = new Set(ids);
    const hit = [...this.rows.values()].filter((r) => wanted.has(r.id)).map((r) => r.id);
    if (hit.length === 0) return;
    let fresh: MessageSummary[];
    try {
      fresh = await this.client.message.summaries({ ids: hit });
    } catch {
      return;
    }
    if (this.closed) return;
    const byId = new Map(fresh.map((s) => [s.id, s]));
    let changed = false;
    for (const [i, row] of this.rows) {
      const s = byId.get(row.id);
      if (s) {
        this.rows.set(i, s);
        changed = true;
      }
    }
    if (changed) this.notify();
  }

  private pageLoaded(p: number): boolean {
    const end = Math.min((p + 1) * PAGE, this.total);
    for (let i = p * PAGE; i < end; i++) if (!this.rows.has(i)) return false;
    return true;
  }

  private evict(): void {
    const lo = (Math.floor(this.window[0] / PAGE) - KEEP_PAGES) * PAGE;
    const hi = (Math.ceil(this.window[1] / PAGE) + KEEP_PAGES) * PAGE;
    for (const i of this.rows.keys()) if (i < lo || i >= hi) this.rows.delete(i);
  }

  private notify(): void {
    this.version++;
    for (const fn of this.listeners) fn();
  }
}
