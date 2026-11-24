// Command testshard prints one shard of a list of Go packages, so that a
// test run can be split over several machines (docs/design/testing.md, CI).
// It reads import paths from standard input, one a line, and prints those
// of shard I of N:
//
//	go list ./... | testshard -shard I/N
//
// The N shards of one list hold every package of it exactly once, whatever
// the weights say: a package that weights.txt does not name still lands in
// a shard. The weights only balance the shards.
package main

import (
	"bufio"
	"cmp"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
)

// weights.txt holds the seconds each slow package's tests take under the
// race detector on CI's runner.
//
//go:embed weights.txt
var weightsFile string

// unlisted is the weight of a package that weights.txt does not name.
const unlisted = 1

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "testshard:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("testshard", flag.ContinueOnError)
	spec := fs.String("shard", "", "the shard to print, as I/N: shard I of N, from 1")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q: packages come from standard input", fs.Arg(0))
	}
	i, n, err := parseShard(*spec)
	if err != nil {
		return err
	}
	weights, err := parseWeights(weightsFile)
	if err != nil {
		return err
	}
	pkgs, err := readPackages(in)
	if err != nil {
		return err
	}
	shards, err := split(pkgs, weights, n)
	if err != nil {
		return err
	}
	for _, p := range shards[i-1] {
		if _, err := fmt.Fprintln(out, p); err != nil {
			return err
		}
	}
	return nil
}

// parseShard reads I/N with 1 <= I <= N.
func parseShard(spec string) (i, n int, err error) {
	a, b, ok := strings.Cut(spec, "/")
	if ok {
		i, err = strconv.Atoi(a)
	}
	if ok && err == nil {
		n, err = strconv.Atoi(b)
	}
	if !ok || err != nil || i < 1 || n < i {
		return 0, 0, fmt.Errorf("-shard %q: want I/N with 1 <= I <= N", spec)
	}
	return i, n, nil
}

// parseWeights reads lines of "seconds import/path"; # starts a comment.
func parseWeights(text string) (map[string]int, error) {
	weights := map[string]int{}
	for n, line := range strings.Split(text, "\n") {
		line, _, _ = strings.Cut(line, "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		var w int
		var err error
		if len(fields) == 2 {
			w, err = strconv.Atoi(fields[0])
		}
		if len(fields) != 2 || err != nil || w < 1 {
			return nil, fmt.Errorf("weights.txt:%d: want a positive number of seconds and an import path", n+1)
		}
		if _, dup := weights[fields[1]]; dup {
			return nil, fmt.Errorf("weights.txt:%d: %s is listed twice", n+1, fields[1])
		}
		weights[fields[1]] = w
	}
	return weights, nil
}

// readPackages reads one import path a line, skipping blank lines.
func readPackages(in io.Reader) ([]string, error) {
	var pkgs []string
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		if p := strings.TrimSpace(sc.Text()); p != "" {
			pkgs = append(pkgs, p)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read packages: %w", err)
	}
	return pkgs, nil
}

// split deals pkgs into n shards: the heaviest package first, each to the
// shard that is lightest so far. The result depends only on the set of
// packages and the weights, so every machine of a run computes the same
// shards. It refuses a list that would leave a shard empty, since a test
// command given no package tests the current directory instead.
func split(pkgs []string, weights map[string]int, n int) ([][]string, error) {
	if n < 1 {
		return nil, errors.New("no shards")
	}
	order := slices.Clone(pkgs)
	slices.Sort(order)
	for i := 1; i < len(order); i++ {
		if order[i] == order[i-1] {
			return nil, fmt.Errorf("package %s is listed twice", order[i])
		}
	}
	if len(order) < n {
		return nil, fmt.Errorf("%d packages cannot fill %d shards", len(order), n)
	}
	weight := func(p string) int {
		if w, ok := weights[p]; ok {
			return w
		}
		return unlisted
	}
	slices.SortStableFunc(order, func(a, b string) int { return cmp.Compare(weight(b), weight(a)) })
	shards := make([][]string, n)
	load := make([]int, n)
	for _, p := range order {
		least := 0
		for s := range load {
			if load[s] < load[least] {
				least = s
			}
		}
		shards[least] = append(shards[least], p)
		load[least] += weight(p)
	}
	for _, s := range shards {
		slices.Sort(s)
	}
	return shards, nil
}
