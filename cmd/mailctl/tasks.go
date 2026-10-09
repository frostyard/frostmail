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

func newTasksCmd(opts *rootOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "tasks", Short: "Inspect and manage tasks"}
	cmd.AddCommand(newTasksLsCmd(opts), newTasksAddCmd(opts), newTasksDoneCmd(opts), newTasksRmCmd(opts))
	return cmd
}

func pimDateFlag(cmd *cobra.Command, name, value string) (*string, error) {
	if !cmd.Flags().Changed(name) {
		return nil, nil
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return nil, fmt.Errorf("invalid %s date %q: %w", name, value, err)
	}
	return &value, nil
}

func newTasksLsCmd(opts *rootOptions) *cobra.Command {
	var list, due string
	var all bool
	cmd := &cobra.Command{
		Use: "ls", Short: "List tasks", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listID, err := pimIDFlag(cmd, "list", list)
			if err != nil {
				return err
			}
			dueBefore, err := pimDateFlag(cmd, "due-before", due)
			if err != nil {
				return err
			}
			return runTasksList(cmd, opts, &api.TasksListParams{ListID: listID, Completed: &all, DueBefore: dueBefore})
		},
	}
	cmd.Flags().StringVar(&list, "list", "", "limit to one task list")
	cmd.Flags().BoolVar(&all, "all", false, "include completed tasks")
	cmd.Flags().StringVar(&due, "due-before", "", "only tasks due before this day (YYYY-MM-DD)")
	return cmd
}

func runTasksList(cmd *cobra.Command, opts *rootOptions, p *api.TasksListParams) error {
	ctx := cmd.Context()
	c, _, err := opts.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	rows, err := c.Tasks().List(ctx, p)
	if err != nil {
		return err
	}
	if written, err := clix.OutputJSON(rows); err != nil || written {
		return err
	}
	kind := api.CollectionKindTasklist
	lists, err := c.Account().Collections(ctx, &api.AccountCollectionsParams{Kind: &kind})
	if err != nil {
		return err
	}
	return printTasks(cmd.OutOrStdout(), rows, lists)
}

func printTasks(w io.Writer, rows []api.Task, lists []api.Collection) error {
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "no tasks")
		return err
	}
	names := make(map[int64]string, len(lists))
	for _, list := range lists {
		names[list.ID] = list.Name
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tDONE\tDUE\tLIST\tTITLE")
	for _, row := range rows {
		done := "-"
		if row.Completed {
			done = "x"
		}
		title := pimCell(row.Title)
		if row.ParentID != nil {
			title = "↳ " + title
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", row.ID, done, pimCell(row.Due), pimCell(names[row.ListID]), title)
	}
	return tw.Flush()
}

func newTasksAddCmd(opts *rootOptions) *cobra.Command {
	var list, due, notes, parent string
	cmd := &cobra.Command{
		Use: "add TITLE...", Short: "Add a task", Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listID, err := pimIDFlag(cmd, "list", list)
			if err != nil {
				return err
			}
			parentID, err := pimIDFlag(cmd, "parent", parent)
			if err != nil {
				return err
			}
			dueDate, err := pimDateFlag(cmd, "due", due)
			if err != nil {
				return err
			}
			p := &api.TasksCreateParams{Title: strings.Join(args, " "), ListID: listID, ParentID: parentID, Due: dueDate}
			if cmd.Flags().Changed("notes") {
				p.Notes = &notes
			}
			return runTasksCreate(cmd, opts, p)
		},
	}
	cmd.Flags().StringVar(&list, "list", "", "task list ID")
	cmd.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&notes, "notes", "", "task notes")
	cmd.Flags().StringVar(&parent, "parent", "", "parent task ID")
	return cmd
}

func runTasksCreate(cmd *cobra.Command, opts *rootOptions, p *api.TasksCreateParams) error {
	ctx := cmd.Context()
	c, _, err := opts.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	task, err := c.Tasks().Create(ctx, p)
	if err != nil {
		return err
	}
	if written, err := clix.OutputJSON(task); err != nil || written {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "task %d\n", task.ID)
	return err
}

func newTasksDoneCmd(opts *rootOptions) *cobra.Command {
	var undo bool
	cmd := &cobra.Command{
		Use: "done ID", Short: "Complete or reopen a task", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pimID(args[0], "task")
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			completed := !undo
			task, err := c.Tasks().Update(ctx, &api.TasksUpdateParams{ID: id, Completed: &completed})
			if err != nil {
				return err
			}
			_, err = clix.OutputJSON(task)
			return err
		},
	}
	cmd.Flags().BoolVar(&undo, "undo", false, "reopen the task")
	return cmd
}

func newTasksRmCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use: "rm ID", Short: "Delete a task and its subtasks", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := pimID(args[0], "task")
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			c, _, err := opts.dial(ctx)
			if err != nil {
				return err
			}
			defer c.Close()
			if err := c.Tasks().Delete(ctx, &api.TasksDeleteParams{ID: id}); err != nil {
				return err
			}
			_, err = clix.OutputJSON(nil)
			return err
		},
	}
}
