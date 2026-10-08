// Command searchbench times searches the way the app runs them: view.open
// with the search text over maild's socket, which builds the view's whole
// snapshot (docs/plans/0006-m4-daily-driver.md, exit evidence 3).
//
//	go run ./tools/searchbench -socket build/x/run/maild.sock -runs 20 [QUERY...]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
)

// defaultQueries cover the language: words, prefixes, phrases, columns,
// negation, flags, dates and mailboxes.
var defaultQueries = []string{
	"ledger",
	"le",
	`"harbor market"`,
	"from:maria",
	"subject:router -kernel",
	"is:unread has:attachment",
	"newer_than:30d beacon",
	"in:inbox from:walker canvas",
	"zzzznotaword",
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "searchbench:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("searchbench", flag.ContinueOnError)
	socket := fs.String("socket", "", "maild's socket (required)")
	runs := fs.Int("runs", 20, "timed runs per query, after one warm-up")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *socket == "" {
		return errors.New("-socket is required")
	}
	queries := fs.Args()
	if len(queries) == 0 {
		queries = defaultQueries
	}
	ctx := context.Background()
	c, _, err := api.Dial(ctx, *socket, "searchbench")
	if err != nil {
		return err
	}
	defer c.Close()
	var all []time.Duration
	fmt.Printf("%-28s %8s %8s %8s %8s\n", "query", "results", "p50", "p95", "max")
	for _, q := range queries {
		times := make([]time.Duration, 0, *runs)
		var count int64
		for i := range *runs + 1 {
			text := q
			start := time.Now()
			v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Text: &text}})
			if err != nil {
				return fmt.Errorf("%q: %w", q, err)
			}
			if _, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 50}); err != nil {
				return fmt.Errorf("%q: %w", q, err)
			}
			took := time.Since(start)
			_ = c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID})
			count = v.Count
			if i > 0 {
				times = append(times, took)
			}
		}
		all = append(all, times...)
		slices.Sort(times)
		fmt.Printf("%-28s %8d %8s %8s %8s\n", q, count, pct(times, 50), pct(times, 95), times[len(times)-1].Round(time.Millisecond))
	}
	slices.Sort(all)
	fmt.Printf("%-28s %8s %8s %8s %8s\n", "all", "", pct(all, 50), pct(all, 95), all[len(all)-1].Round(time.Millisecond))
	return nil
}

// pct is the p-th percentile of sorted durations, nearest rank.
func pct(sorted []time.Duration, p int) time.Duration {
	i := max(0, (len(sorted)*p+99)/100-1)
	return sorted[i].Round(100 * time.Microsecond)
}
