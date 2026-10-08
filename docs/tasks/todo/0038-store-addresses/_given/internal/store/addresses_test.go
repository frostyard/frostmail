package store

// CONTRACT TEST for task card T-0038 (docs/tasks). Do not edit.

import (
	"reflect"
	"testing"
	"time"
)

func record(t *testing.T, d *DB, seen time.Time, as ...Address) {
	t.Helper()
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.RecordAddresses(t.Context(), as, seen) }); err != nil {
		t.Fatal(err)
	}
}

type addrRow struct {
	name     string
	count    int
	lastSeen string
}

func addrRows(t *testing.T, d *DB) map[string]addrRow {
	t.Helper()
	rows, err := d.db.QueryContext(t.Context(), `SELECT address, name, count, last_seen FROM addresses`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]addrRow{}
	for rows.Next() {
		var a string
		var r addrRow
		if err := rows.Scan(&a, &r.name, &r.count, &r.lastSeen); err != nil {
			t.Fatal(err)
		}
		out[a] = r
	}
	return out
}

func TestRecordAddresses(t *testing.T) {
	d, _ := openTest(t)
	t1 := testNow
	record(t, d, t1, Address{Name: "Ann", Addr: "Ann@X.test"}, Address{Addr: "bob@x.test"}, Address{Addr: "ann@x.test"}, Address{Name: "Nobody"})
	got := addrRows(t, d)
	want := map[string]addrRow{
		"ann@x.test": {name: "Ann", count: 1, lastSeen: FormatTime(t1)},
		"bob@x.test": {name: "", count: 1, lastSeen: FormatTime(t1)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("after one call:\n got %+v\nwant %+v (lowercased, once per call, empty addresses skipped)", got, want)
	}
	t2 := testNow.Add(time.Hour)
	record(t, d, t2, Address{Addr: "ann@x.test"})
	record(t, d, testNow.Add(-time.Hour), Address{Name: "Ann Smith", Addr: "ANN@x.test"})
	got = addrRows(t, d)
	if r := got["ann@x.test"]; r.count != 3 || r.name != "Ann Smith" || r.lastSeen != FormatTime(t2) {
		t.Fatalf("ann = %+v; want count 3, the latest non-empty name, the latest time", r)
	}
}

func TestSuggestAddresses(t *testing.T) {
	d, _ := openTest(t)
	at := func(m int) time.Time { return testNow.Add(time.Duration(m) * time.Minute) }
	record(t, d, at(1), Address{Name: "Anna Lee", Addr: "anna@x.test"})
	for range 3 {
		record(t, d, at(2), Address{Name: "Andrew Kim", Addr: "andrew@x.test"})
	}
	for range 3 {
		record(t, d, at(3), Address{Name: "Ann Smith", Addr: "ann@x.test"})
	}
	record(t, d, at(4), Address{Name: "Bob Annan", Addr: "bob@x.test"})
	record(t, d, at(5), Address{Addr: "under_score@x.test"})
	cases := []struct {
		prefix string
		limit  int
		want   []string
	}{
		{"an", 0, []string{"ann@x.test", "andrew@x.test", "bob@x.test", "anna@x.test"}},
		{"AN", 0, []string{"ann@x.test", "andrew@x.test", "bob@x.test", "anna@x.test"}},
		{"an", 2, []string{"ann@x.test", "andrew@x.test"}},
		{"smi", 0, []string{"ann@x.test"}},
		{"kim", 0, []string{"andrew@x.test"}},
		{"bob@", 0, []string{"bob@x.test"}},
		{"x.test", 0, nil},
		{"", 0, nil},
		{"_", 0, nil},
		{"%", 0, nil},
		{"under_", 0, []string{"under_score@x.test"}},
	}
	for _, tc := range cases {
		got, err := d.SuggestAddresses(t.Context(), tc.prefix, tc.limit)
		if err != nil {
			t.Fatalf("%q: %v", tc.prefix, err)
		}
		var addrs []string
		for _, a := range got {
			addrs = append(addrs, a.Addr)
		}
		if !reflect.DeepEqual(addrs, tc.want) {
			t.Errorf("SuggestAddresses(%q, %d) = %v, want %v", tc.prefix, tc.limit, addrs, tc.want)
		}
	}
	got, _ := d.SuggestAddresses(t.Context(), "smi", 0)
	if len(got) != 1 || got[0].Name != "Ann Smith" {
		t.Fatalf("suggestion = %+v; want the name too", got)
	}
}

func TestSuggestAddressesDefaultLimit(t *testing.T) {
	d, _ := openTest(t)
	for i := range 15 {
		record(t, d, testNow, Address{Addr: string(rune('a'+i)) + "z@x.test"}, Address{Addr: "z" + string(rune('a'+i)) + "@x.test"})
	}
	got, err := d.SuggestAddresses(t.Context(), "z", 0)
	if err != nil || len(got) != 10 {
		t.Fatalf("default limit: %d results, %v; want 10", len(got), err)
	}
}
