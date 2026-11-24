package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The shards of a list are a partition of it: every package in exactly one
// shard, whether weights.txt names it or not, for every number of shards.
func TestShardsHoldEveryPackageOnce(t *testing.T) {
	weights := map[string]int{"m/heavy": 200, "m/big": 90, "m/mid": 40, "m/gone": 500}
	var pkgs []string
	for i := range 23 {
		pkgs = append(pkgs, fmt.Sprintf("m/p%02d", i))
	}
	pkgs = append(pkgs, "m/heavy", "m/big", "m/mid")
	for n := 1; n <= len(pkgs); n++ {
		shards, err := split(pkgs, weights, n)
		if err != nil {
			t.Fatalf("%d shards: %v", n, err)
		}
		if len(shards) != n {
			t.Fatalf("%d shards: got %d", n, len(shards))
		}
		var all []string
		for i, s := range shards {
			if len(s) == 0 {
				t.Errorf("%d shards: shard %d is empty", n, i+1)
			}
			all = append(all, s...)
		}
		slices.Sort(all)
		want := slices.Sorted(slices.Values(pkgs))
		if !slices.Equal(all, want) {
			t.Errorf("%d shards: together they hold %v, want %v", n, all, want)
		}
	}
}

// The order of the input does not change the shards, so the machines of one
// run agree on them.
func TestShardsDoNotDependOnInputOrder(t *testing.T) {
	weights := map[string]int{"m/a": 7, "m/b": 7, "m/c": 3}
	pkgs := []string{"m/a", "m/b", "m/c", "m/d", "m/e", "m/f", "m/g"}
	want, err := split(pkgs, weights, 3)
	if err != nil {
		t.Fatal(err)
	}
	back := slices.Clone(pkgs)
	slices.Reverse(back)
	got, err := split(back, weights, 3)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("reversed input gives %v, want %v", got, want)
	}
}

// The heaviest packages go to different shards, and light ones fill up the
// shards that hold less.
func TestShardsBalanceByWeight(t *testing.T) {
	weights := map[string]int{"m/a": 100, "m/b": 90, "m/c": 50, "m/d": 40}
	shards, err := split([]string{"m/a", "m/b", "m/c", "m/d", "m/e"}, weights, 2)
	if err != nil {
		t.Fatal(err)
	}
	// a:100 | b:90, then c to the second (140), d to the first (140), and
	// e, unlisted, to the first of two equal shards.
	want := [][]string{{"m/a", "m/d", "m/e"}, {"m/b", "m/c"}}
	if fmt.Sprint(shards) != fmt.Sprint(want) {
		t.Errorf("shards are %v, want %v", shards, want)
	}
}

func TestSplitRefusesWhatWouldDropOrRepeatAPackage(t *testing.T) {
	if _, err := split([]string{"m/a", "m/b"}, nil, 3); err == nil {
		t.Error("2 packages in 3 shards: no error, though a shard is empty")
	}
	if _, err := split(nil, nil, 1); err == nil {
		t.Error("no packages: no error")
	}
	if _, err := split([]string{"m/a", "m/b", "m/a"}, nil, 2); err == nil {
		t.Error("a package listed twice: no error")
	}
}

func TestParseShard(t *testing.T) {
	for spec, want := range map[string][2]int{"1/1": {1, 1}, "2/4": {2, 4}, "4/4": {4, 4}} {
		i, n, err := parseShard(spec)
		if err != nil || i != want[0] || n != want[1] {
			t.Errorf("parseShard(%q) = %d, %d, %v", spec, i, n, err)
		}
	}
	for _, spec := range []string{"", "1", "0/4", "5/4", "a/4", "1/b", "1/0", "-1/4", "1/4/2"} {
		if _, _, err := parseShard(spec); err == nil {
			t.Errorf("parseShard(%q): no error", spec)
		}
	}
}

func TestParseWeights(t *testing.T) {
	got, err := parseWeights("# seconds\n12\tm/a\n\n3 m/b # slow\n")
	if err != nil || len(got) != 2 || got["m/a"] != 12 || got["m/b"] != 3 {
		t.Errorf("parseWeights = %v, %v", got, err)
	}
	for _, bad := range []string{"m/a\n", "x m/a\n", "0 m/a\n", "1 m/a extra\n", "1 m/a\n2 m/a\n"} {
		if _, err := parseWeights(bad); err == nil {
			t.Errorf("parseWeights(%q): no error", bad)
		}
	}
}

func TestRunPrintsTheShard(t *testing.T) {
	in := "m/b\n\nm/a\nm/c\n"
	var all []string
	for _, shard := range []string{"1/2", "2/2"} {
		var out bytes.Buffer
		if err := run([]string{"-shard", shard}, strings.NewReader(in), &out); err != nil {
			t.Fatalf("shard %s: %v", shard, err)
		}
		all = append(all, strings.Fields(out.String())...)
	}
	slices.Sort(all)
	if want := []string{"m/a", "m/b", "m/c"}; !slices.Equal(all, want) {
		t.Errorf("the two shards hold %v, want %v", all, want)
	}
	if err := run([]string{"-shard", "1/2", "./..."}, strings.NewReader(in), &bytes.Buffer{}); err == nil {
		t.Error("a package argument: no error")
	}
	if err := run(nil, strings.NewReader(in), &bytes.Buffer{}); err == nil {
		t.Error("no -shard: no error")
	}
}

// weights.txt parses, and each package it names is one of this module's:
// a renamed or removed package must leave the file, so that the weights
// stay a record of real packages. The race gate runs on Linux, so the
// packages are Linux's wherever this test runs (Windows has no scripts
// package).
func TestWeightsNameThisModulesPackages(t *testing.T) {
	weights, err := parseWeights(weightsFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(weights) == 0 {
		t.Fatal("weights.txt names no package")
	}
	cmd := exec.CommandContext(t.Context(), "go", "list", "../../...")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	pkgs := strings.Fields(string(out))
	for p := range weights {
		if !slices.Contains(pkgs, p) {
			t.Errorf("weights.txt names %s, which go list ./... does not", p)
		}
	}
}
