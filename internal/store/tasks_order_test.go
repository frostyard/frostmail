package store

import (
	"slices"
	"testing"
)

// Tasks made here in one clock tick share a local position (Windows' clock
// is coarse): the later one comes first, as localPosition intends, ahead
// of Google's positions; Google's own ties still go by title.
func TestLocalPositionTiesPutTheLaterTaskFirst(t *testing.T) {
	rows := []TaskRow{
		{ObjectID: 1, Title: "Call Ann", Position: "0/0000000000000000009"},
		{ObjectID: 3, Title: "Zeta", Position: "00000000000000000001"},
		{ObjectID: 2, Title: "Offsite", Position: "0/0000000000000000009"},
		{ObjectID: 4, Title: "Alpha", Position: "00000000000000000001"},
	}
	slices.SortFunc(rows, compareTasks)
	var got []string
	for _, r := range rows {
		got = append(got, r.Title)
	}
	if want := []string{"Offsite", "Call Ann", "Alpha", "Zeta"}; !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}
