package main

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/frostyard/clix"
	"github.com/frostyard/frostmail/api"
	"github.com/spf13/cobra"
)

func newRulesCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "rules", Short: "Inspect and manage mail rules"}
	cmd.AddCommand(newRulesLsCmd(opts), newRulesEnabledCmd(opts, "on", true),
		newRulesEnabledCmd(opts, "off", false), newRulesRmCmd(opts),
		newRulesMvCmd(opts), newRulesApplyCmd(opts), newRulesImportCmd(opts))
	return cmd
}

func ruleWord(n int) string {
	if n == 1 {
		return "rule"
	}
	return "rules"
}

func newRulesLsCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "ls", Short: "List rules", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			rows, err := c.Rule().List(ctx, &api.RuleListParams{})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(rows); err != nil || written {
				return err
			}
			return printRules(cmd.OutOrStdout(), rows)
		},
	}
}

func printRules(w io.Writer, rows []api.Rule) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no rules")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "#\tID\tON\tNAME\tPROBLEM")
	for _, row := range rows {
		on, problem := "-", ""
		if row.Enabled {
			on = "x"
		}
		if row.Problem != nil {
			problem = *row.Problem
		}
		fmt.Fprintf(tw, "%d\t%d\t%s\t%s\t%s\n", row.Position+1, row.ID, on, row.Name, pimCell(problem))
	}
	return tw.Flush()
}

func newRulesEnabledCmd(opts *rootOptions, name string, enabled bool) *cobra.Command {
	return &cobra.Command{
		Use: name + " RULE_ID...", Short: "Turn rules " + name, Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("rule", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			rows := make([]api.Rule, 0, len(ids))
			for _, id := range ids {
				row, err := c.Rule().Update(ctx, &api.RuleUpdateParams{ID: id, Enabled: &enabled})
				if err != nil {
					return err
				}
				rows = append(rows, *row)
			}
			if written, err := clix.OutputJSON(rows); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "updated %d %s\n", len(ids), ruleWord(len(ids)))
			return err
		},
	}
}

func newRulesRmCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "rm RULE_ID...", Short: "Remove rules", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("rule", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			for _, id := range ids {
				if err := c.Rule().Delete(ctx, &api.RuleDeleteParams{ID: id}); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "removed %d %s\n", len(ids), ruleWord(len(ids)))
			return err
		},
	}
}

func newRulesMvCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "mv RULE_ID POSITION", Short: "Move a rule to a position (from 1)", Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("rule", args[:1])
			if err != nil {
				return err
			}
			position, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil || position < 1 {
				return fmt.Errorf("invalid position %q: must be a number at least 1", args[1])
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Rule().Move(ctx, &api.RuleMoveParams{ID: ids[0], Position: position - 1}); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "moved rule %d to %d\n", ids[0], position)
			return err
		},
	}
}

func newRulesApplyCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "apply MESSAGE_ID...", Short: "Apply rules to messages", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := parseIDs("message", args)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			result, err := c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: ids})
			if err != nil {
				return err
			}
			if written, err := clix.OutputJSON(result); err != nil || written {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%d of %d %s matched a rule\n", result.Matched, len(ids), messageWord(len(ids)))
			return err
		},
	}
}

func newRulesImportCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "import FILE", Short: "Import a rules JSON array (- for stdin)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r := cmd.InOrStdin()
			if args[0] != "-" {
				f, err := os.Open(args[0])
				if err != nil {
					return fmt.Errorf("open rules file: %w", err)
				}
				defer f.Close()
				r = f
			}
			var rows []api.Rule
			if err := json.UnmarshalRead(r, &rows); err != nil {
				return fmt.Errorf("read rules file: %w", err)
			}
			if len(rows) == 0 {
				return errors.New("rules file contains no rules")
			}
			return runRulesImport(cmd, opts, rows)
		},
	}
}

func runRulesImport(cmd *cobra.Command, opts *rootOptions, rows []api.Rule) error {
	ctx := cmd.Context()
	c, _, err := opts.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	for i, row := range rows {
		_, err := c.Rule().Create(ctx, &api.RuleCreateParams{
			Name: row.Name, Conditions: row.Conditions, Actions: row.Actions, Enabled: &row.Enabled,
		})
		if err != nil {
			return fmt.Errorf("rule %d %q: added %d %s before it: %w", i+1, row.Name, i, ruleWord(i), err)
		}
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "added %d %s\n", len(rows), ruleWord(len(rows)))
	return err
}
