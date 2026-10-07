// The view delta algorithm for MockTransport, with maild's semantics
// (internal/view/diff.go): IDs kept in the same relative order stay; every
// other ID is removed and inserted at its new place. Ops apply in order.
import type { ViewOp } from "../gen/api";

/** diffIds returns ops that turn prev into next. */
export function diffIds(prev: readonly number[], next: readonly number[]): ViewOp[] {
  const nextPos = new Map<number, number>();
  next.forEach((id, i) => {
    nextPos.set(id, i);
  });
  const keep = longestIncreasing(prev.map((id) => nextPos.get(id) ?? -1));
  const kept = new Set<number>();
  prev.forEach((id, i) => {
    if (keep[i]) kept.add(id);
  });

  const ops: ViewOp[] = [];
  // Removals from the end, so earlier indices stay valid.
  for (let i = prev.length - 1; i >= 0; ) {
    if (keep[i]) {
      i--;
      continue;
    }
    let start = i;
    while (start > 0 && !keep[start - 1]) start--;
    ops.push({ op: "remove", at: start, count: i - start + 1 });
    i = start - 1;
  }
  // Insertions in ascending order: before index i, the list already matches
  // next[0..i).
  for (let i = 0; i < next.length; ) {
    const id = next[i];
    if (id !== undefined && kept.has(id)) {
      i++;
      continue;
    }
    let end = i;
    while (end < next.length) {
      const e = next[end];
      if (e !== undefined && kept.has(e)) break;
      end++;
    }
    ops.push({ op: "insert", at: i, count: end - i });
    i = end;
  }
  return ops;
}

/** applyOps applies ops to ids, filling inserted slots from next. */
export function applyOps(prev: readonly number[], ops: readonly ViewOp[], next: readonly number[]): number[] {
  const out = [...prev];
  for (const op of ops) {
    if (op.op === "remove") out.splice(op.at, op.count);
    else out.splice(op.at, 0, ...next.slice(op.at, op.at + op.count));
  }
  return out;
}

// longestIncreasing marks a longest strictly increasing subsequence of the
// non-negative values in seq.
function longestIncreasing(seq: readonly number[]): boolean[] {
  const tailIdx: number[] = [];
  const parent = new Array<number>(seq.length).fill(-1);
  seq.forEach((v, i) => {
    if (v < 0) return;
    let lo = 0;
    let hi = tailIdx.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      const t = tailIdx[mid];
      if (t !== undefined && (seq[t] ?? 0) < v) lo = mid + 1;
      else hi = mid;
    }
    if (lo > 0) parent[i] = tailIdx[lo - 1] ?? -1;
    tailIdx[lo] = i;
  });
  const mark = new Array<boolean>(seq.length).fill(false);
  let k = tailIdx[tailIdx.length - 1] ?? -1;
  while (k >= 0) {
    mark[k] = true;
    k = parent[k] ?? -1;
  }
  return mark;
}
