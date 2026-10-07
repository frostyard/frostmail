// Package view keeps live, ordered message lists for clients
// (the view domain in docs/specs/rpc-api.md).
package view

import (
	"github.com/frostyard/frostmail/api"
)

// Diff turns an old view into the ops that produce new. IDs are unique
// within each list. The rows that stay put are a longest strictly
// increasing subsequence of the new indexes of the shared IDs, so a small
// edit stays a small delta. Removes come first, from the end of old toward
// the start, so earlier indexes stay valid; then inserts, from the start
// of new toward the end, where each run lands exactly at its new index.
// Diff returns nil when nothing changed.
func Diff(old, new []int64) []api.ViewOp {
	pos := make(map[int64]int, len(new))
	for i, id := range new {
		pos[id] = i
	}

	// The new index of every shared ID, in old order.
	seq := make([]int, 0, len(old))
	oldAt := make([]int, 0, len(old))
	for i, id := range old {
		if j, ok := pos[id]; ok {
			seq = append(seq, j)
			oldAt = append(oldAt, i)
		}
	}
	keptOld, keptNew := keptMask(seq, oldAt, len(old), len(new))

	var ops []api.ViewOp
	for i := len(old) - 1; i >= 0; i-- {
		if keptOld[i] {
			continue
		}
		j := i
		for j >= 0 && !keptOld[j] {
			j--
		}
		ops = append(ops, api.ViewOp{Op: api.ViewOpKindRemove, At: int64(j + 1), Count: int64(i - j)})
		i = j + 1
	}
	for i := 0; i < len(new); i++ {
		if keptNew[i] {
			continue
		}
		j := i
		for j < len(new) && !keptNew[j] {
			j++
		}
		ops = append(ops, api.ViewOp{Op: api.ViewOpKindInsert, At: int64(i), Count: int64(j - i)})
		i = j - 1
	}
	return ops
}

// keptMask marks, for the old and the new list, the rows that belong to a
// longest strictly increasing subsequence of seq. seq holds the new index
// of each shared row in old order, with oldAt lining up index for index.
// The subsequence is found by patience sorting with binary search, keeping
// predecessor links to walk the winner back out.
func keptMask(seq, oldAt []int, lenOld, lenNew int) (keptOld, keptNew []bool) {
	keptOld = make([]bool, lenOld)
	keptNew = make([]bool, lenNew)
	tails := make([]int, 0, len(seq))
	prev := make([]int, len(seq))
	for i, v := range seq {
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if seq[tails[mid]] < v {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		prev[i] = -1
		if lo > 0 {
			prev[i] = tails[lo-1]
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			keptOld[oldAt[i]] = true
			keptNew[seq[i]] = true
		}
	}
	return keptOld, keptNew
}
