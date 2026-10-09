package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

func newPeopleCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "people", Short: "Inspect contacts"}
	cmd.AddCommand(newPeopleLsCmd(opts), newPeopleShowCmd(opts))
	return cmd
}

func pimCell(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func pimID(s, kind string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s id %q: %w", kind, s, err)
	}
	return id, nil
}

func pimIDFlag(cmd *cobra.Command, name, value string) (*int64, error) {
	if !cmd.Flags().Changed(name) {
		return nil, nil
	}
	id, err := pimID(value, name)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func newPeopleLsCmd(opts *rootOptions) *cobra.Command {
	var book string
	cmd := &cobra.Command{
		Use: "ls [TERMS...]", Short: "List or search people",
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID, err := pimIDFlag(cmd, "book", book)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			query := strings.Join(args, " ")
			rows, err := c.People().List(ctx, &api.PeopleListParams{Query: &query, CollectionID: bookID})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(rows); err != nil || written {
				return err
			}
			return printPeople(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&book, "book", "", "limit to one address book")
	return cmd
}

func printPeople(w io.Writer, rows []api.PersonSummary) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no people")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tEMAIL\tORGANIZATION")
	for _, row := range rows {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", row.ID, pimCell(row.DisplayName), pimCell(row.Email), pimCell(row.Organization))
	}
	return tw.Flush()
}

func newPeopleShowCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "show ID", Short: "Show one person's contacts", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pimID(args[0], "person")
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			person, err := c.People().Get(ctx, &api.PeopleGetParams{ID: id})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(person); err != nil || written {
				return err
			}
			return printPerson(cmd.OutOrStdout(), person)
		},
	}
}

func printPerson(w io.Writer, person *api.Person) error {
	var b strings.Builder
	fmt.Fprintln(&b, person.DisplayName)
	if person.Organization != "" {
		fmt.Fprintf(&b, "Organization: %s\n", person.Organization)
	}
	for _, contact := range person.Contacts {
		printContactValues(&b, "Email", contact.Emails)
		printContactValues(&b, "Phone", contact.Phones)
		if contact.Note != "" {
			fmt.Fprintf(&b, "Note: %s\n", contact.Note)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func printContactValues(b *strings.Builder, kind string, values []api.LabeledValue) {
	for _, value := range values {
		label := ""
		if value.Label != "" {
			label = " (" + value.Label + ")"
		}
		fmt.Fprintf(b, "%s: %s%s\n", kind, value.Value, label)
	}
}
