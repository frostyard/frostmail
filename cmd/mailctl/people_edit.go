package main

import (
	"fmt"
	"strings"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

type peopleFieldOptions struct {
	first, last, nickname, company, title, note, birthday string
	emails, phones                                        []string
}

func (o *peopleFieldOptions) flags(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&o.first, "first", "", "given name")
	f.StringVar(&o.last, "last", "", "family name")
	f.StringVar(&o.nickname, "nickname", "", "nickname")
	f.StringVar(&o.company, "company", "", "organization")
	f.StringVar(&o.title, "title", "", "job title")
	f.StringVar(&o.note, "note", "", "contact note")
	f.StringVar(&o.birthday, "birthday", "", "birthday (YYYY-MM-DD or --MM-DD; empty to clear)")
	f.StringArrayVar(&o.emails, "email", nil, "email VALUE or LABEL=VALUE (repeatable; replaces all emails)")
	f.StringArrayVar(&o.phones, "phone", nil, "phone VALUE or LABEL=VALUE (repeatable; replaces all phones)")
}

func (o *peopleFieldOptions) fields(cmd *cobra.Command) (api.ContactFields, bool) {
	var fields api.ContactFields
	var changed bool
	for _, field := range []struct {
		name  string
		value *string
		dest  **string
	}{
		{"first", &o.first, &fields.GivenName},
		{"last", &o.last, &fields.FamilyName},
		{"nickname", &o.nickname, &fields.Nickname},
		{"company", &o.company, &fields.Organization},
		{"title", &o.title, &fields.Title},
		{"note", &o.note, &fields.Note},
		{"birthday", &o.birthday, &fields.Birthday},
	} {
		if cmd.Flags().Changed(field.name) {
			*field.dest = field.value
			changed = true
		}
	}
	if cmd.Flags().Changed("email") {
		fields.Emails = peopleLabeledValues(o.emails)
		changed = true
	}
	if cmd.Flags().Changed("phone") {
		fields.Phones = peopleLabeledValues(o.phones)
		changed = true
	}
	return fields, changed
}

func peopleLabeledValues(values []string) []api.LabeledValue {
	result := make([]api.LabeledValue, 0, len(values))
	for _, raw := range values {
		label, value, labeled := strings.Cut(raw, "=")
		if !labeled {
			label, value = "", raw
		}
		result = append(result, api.LabeledValue{Label: strings.TrimSpace(label), Value: strings.TrimSpace(value)})
	}
	return result
}

func newPeopleAddCmd(opts *rootOptions) *cobra.Command {
	var book string
	var fields peopleFieldOptions
	cmd := &cobra.Command{
		Use: "add [--book ID] [flags]", Short: "Add a contact", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bookID, err := pimIDFlag(cmd, "book", book)
			if err != nil {
				return err
			}
			patch, _ := fields.fields(cmd)
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := c.People().Create(ctx, &api.PeopleCreateParams{CollectionID: bookID, Fields: patch})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "added %s (contact %d)\n", result.Person.DisplayName, result.ContactID)
			return err
		},
	}
	cmd.Flags().StringVar(&book, "book", "", "address book ID")
	fields.flags(cmd)
	return cmd
}

func newPeopleEditCmd(opts *rootOptions) *cobra.Command {
	var fields peopleFieldOptions
	cmd := &cobra.Command{
		Use: "edit CONTACT_ID [flags]", Short: "Edit a contact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pimID(args[0], "contact")
			if err != nil {
				return err
			}
			patch, changed := fields.fields(cmd)
			if !changed {
				return fmt.Errorf("at least one contact field flag is required")
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := c.People().Update(ctx, &api.PeopleUpdateParams{ID: id, Fields: patch})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", result.Person.DisplayName)
			return err
		},
	}
	fields.flags(cmd)
	return cmd
}

func newPeopleRmCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "rm CONTACT_ID", Short: "Delete a contact", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pimID(args[0], "contact")
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := c.People().Delete(ctx, &api.PeopleDeleteParams{ID: id})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted contact %d\n", result.ContactID)
			return err
		},
	}
}
