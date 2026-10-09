package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

func newCalCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "cal", Short: "Inspect calendars and events"}
	cmd.AddCommand(newCalLsCmd(opts), newCalAgendaCmd(opts))
	return cmd
}

func newCalLsCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "ls", Short: "List calendars", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			kind := api.CollectionKindCalendar
			rows, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(rows); err != nil || written {
				return err
			}
			return printCalendars(cmd.OutOrStdout(), rows)
		},
	}
}

func printCalendars(w io.Writer, rows []api.Collection) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tACCOUNT\tNAME\tSHOWN\tREAD-ONLY")
	for _, row := range rows {
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", row.ID, row.AccountID, pimCell(row.Name), pimYesNo(row.Enabled), pimYesNo(row.ReadOnly))
	}
	return tw.Flush()
}

func pimYesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func agendaRange(from, zone string, days int) (*api.CalendarRangeParams, *time.Location, error) {
	if days < 1 || days > 400 {
		return nil, nil, fmt.Errorf("days must be between 1 and 400")
	}
	loc := time.Local
	if zone != "" {
		var err error
		if loc, err = time.LoadLocation(zone); err != nil {
			return nil, nil, fmt.Errorf("invalid time zone %q: %w", zone, err)
		}
	}
	if from == "" {
		from = time.Now().In(loc).Format(time.DateOnly)
	}
	start, err := time.ParseInLocation(time.DateOnly, from, loc)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid date %q: %w", from, err)
	}
	p := &api.CalendarRangeParams{From: from, To: start.AddDate(0, 0, days).Format(time.DateOnly)}
	if zone != "" && zone != "Local" {
		p.TimeZone = &zone // else maild reads the days in its own zone
	}
	return p, loc, nil
}

func newCalAgendaCmd(opts *rootOptions) *cobra.Command {
	var from, zone string
	var days int
	cmd := &cobra.Command{
		Use: "agenda", Short: "Show upcoming events", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, loc, err := agendaRange(from, zone, days)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			rows, err := c.Calendar().Range(ctx, p)
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(rows); err != nil || written {
				return err
			}
			return printAgenda(cmd.OutOrStdout(), rows, loc)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "first day (YYYY-MM-DD; default: today)")
	cmd.Flags().IntVar(&days, "days", 7, "number of days")
	cmd.Flags().StringVar(&zone, "zone", "", "time zone (IANA; default: the local zone)")
	return cmd
}

func printAgenda(w io.Writer, rows []api.Occurrence, loc *time.Location) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no events")
		return err
	}
	var b strings.Builder
	seen := make(map[string]bool)
	for _, row := range rows {
		start := row.Start.In(loc)
		if row.AllDay {
			var err error
			start, err = time.ParseInLocation(time.DateOnly, row.StartDate, loc)
			if err != nil {
				return fmt.Errorf("invalid event start date %q: %w", row.StartDate, err)
			}
		}
		day := start.Format(time.DateOnly)
		if !seen[day] {
			fmt.Fprintln(&b, start.Format("2006-01-02 Mon"))
			seen[day] = true
		}
		span := "all day    "
		if !row.AllDay {
			span = start.Format("15:04") + "–" + row.End.In(loc).Format("15:04")
		}
		summary := row.Summary
		if row.Location != "" {
			summary += " (" + row.Location + ")"
		}
		if row.Status == api.EventStatusCancelled {
			summary += " [cancelled]"
		}
		fmt.Fprintf(&b, "  %s  %s\n", span, summary)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
