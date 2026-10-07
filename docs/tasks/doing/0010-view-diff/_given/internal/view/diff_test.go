package view

// CONTRACT TEST for task card T-0010 (docs/tasks). Do not edit.

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// apply runs ops on old the way a client does: removes delete rows, inserts
// add placeholder rows (0) that the client then fetches. The result must have
// new's length, and every non-placeholder row must equal new's row there.
func apply(t *testing.T, old, new []int64, ops []api.ViewOp) {
	t.Helper()
	list := slices.Clone(old)
	for i, op := range ops {
		if op.Count <= 0 || op.At < 0 {
			t.Fatalf("op %d = %+v: count must be positive and at non-negative", i, op)
		}
		at, n := int(op.At), int(op.Count)
		switch op.Op {
		case api.ViewOpKindRemove:
			if at+n > len(list) {
				t.Fatalf("op %d = %+v removes past the end of %d rows", i, op, len(list))
			}
			list = slices.Delete(list, at, at+n)
		case api.ViewOpKindInsert:
			if at > len(list) {
				t.Fatalf("op %d = %+v inserts past the end of %d rows", i, op, len(list))
			}
			list = slices.Insert(list, at, make([]int64, n)...)
		default:
			t.Fatalf("op %d has kind %q", i, op.Op)
		}
	}
	if len(list) != len(new) {
		t.Fatalf("after %v: %d rows, want %d", ops, len(list), len(new))
	}
	for i := range list {
		if list[i] != 0 && list[i] != new[i] {
			t.Fatalf("after %v: row %d = %d, want %d or a placeholder", ops, i, list[i], new[i])
		}
	}
}

func ins(at, n int64) api.ViewOp { return api.ViewOp{Op: api.ViewOpKindInsert, At: at, Count: n} }
func rem(at, n int64) api.ViewOp { return api.ViewOp{Op: api.ViewOpKindRemove, At: at, Count: n} }

func TestDiffSimpleCases(t *testing.T) {
	cases := []struct {
		name     string
		old, new []int64
		want     []api.ViewOp
	}{
		{"unchanged", []int64{1, 2, 3}, []int64{1, 2, 3}, nil},
		{"both empty", nil, nil, nil},
		{"from empty", nil, []int64{1, 2}, []api.ViewOp{ins(0, 2)}},
		{"to empty", []int64{1, 2}, nil, []api.ViewOp{rem(0, 2)}},
		{"prepend", []int64{2, 3}, []int64{1, 2, 3}, []api.ViewOp{ins(0, 1)}},
		{"append", []int64{1, 2}, []int64{1, 2, 3}, []api.ViewOp{ins(2, 1)}},
		{"remove a middle run", []int64{1, 2, 3, 4, 5}, []int64{1, 5}, []api.ViewOp{rem(1, 3)}},
		{"replace one", []int64{1, 2, 3}, []int64{1, 9, 3}, []api.ViewOp{rem(1, 1), ins(1, 1)}},
		{"two separate removes", []int64{1, 2, 3, 4, 5}, []int64{1, 3, 5}, []api.ViewOp{rem(3, 1), rem(1, 1)}},
		{"two separate inserts", []int64{1, 3, 5}, []int64{1, 2, 3, 4, 5}, []api.ViewOp{ins(1, 1), ins(3, 1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.old, tc.new)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("Diff(%v, %v) = %v, want %v", tc.old, tc.new, got, tc.want)
			}
			apply(t, tc.old, tc.new, got)
		})
	}
}

func TestDiffMoves(t *testing.T) {
	cases := []struct {
		name     string
		old, new []int64
		maxOps   int
	}{
		{"move first to last", []int64{1, 2, 3, 4}, []int64{2, 3, 4, 1}, 2},
		{"move last to first", []int64{1, 2, 3, 4}, []int64{4, 1, 2, 3}, 2},
		{"swap neighbors", []int64{1, 2, 3, 4}, []int64{1, 3, 2, 4}, 2},
		{"reverse", []int64{1, 2, 3, 4, 5}, []int64{5, 4, 3, 2, 1}, 8},
		{"disjoint", []int64{1, 2, 3}, []int64{4, 5, 6}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.old, tc.new)
			apply(t, tc.old, tc.new, got)
			if len(got) > tc.maxOps {
				t.Fatalf("Diff(%v, %v) = %v: more than %d ops", tc.old, tc.new, got, tc.maxOps)
			}
		})
	}
}

func TestDiffRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for round := range 200 {
		pool := r.Perm(40)
		old := make([]int64, 0, 30)
		for _, v := range pool[:r.IntN(30)] {
			old = append(old, int64(v+1))
		}
		new := slices.Clone(old)
		for range r.IntN(6) {
			switch r.IntN(3) {
			case 0:
				new = slices.Insert(new, r.IntN(len(new)+1), int64(100+round*10+r.IntN(10)))
			case 1:
				if len(new) > 0 {
					i := r.IntN(len(new))
					new = slices.Delete(new, i, i+1)
				}
			case 2:
				if len(new) > 1 {
					i, j := r.IntN(len(new)), r.IntN(len(new))
					new[i], new[j] = new[j], new[i]
				}
			}
		}
		new = slices.Compact(new) // inserted IDs may repeat; IDs in a view are unique
		apply(t, old, new, Diff(old, new))
	}
}

func TestDiffLargeIsFast(t *testing.T) {
	const n = 50_000
	old := make([]int64, n)
	for i := range old {
		old[i] = int64(n - i) // newest first
	}
	new := slices.Clone(old)
	new = slices.Insert(new, 0, n+1, n+2)        // two new arrivals
	new = slices.Delete(new, 20_000, 20_010)     // ten expunged
	new = slices.Insert(new, 30_000, int64(n+3)) // one moved in
	start := time.Now()
	ops := Diff(old, new)
	elapsed := time.Since(start)
	apply(t, old, new, ops)
	if len(ops) != 3 {
		t.Fatalf("ops = %v, want 3", ops)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Diff of %d rows took %v", n, elapsed)
	}
}
