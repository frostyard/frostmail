// Command taskrun runs task cards (docs/tasks) through a local executor
// model and enforces the card's contract: only the listed files change, the
// given tests stay byte-identical, the acceptance command and make check pass.
// See docs/design/agent-workflow.md.
//
//	taskrun list
//	taskrun start ID      branch task/ID, copy given files, commit "T-ID: start"
//	taskrun run ID        start (if needed), run the executor, then verify
//	taskrun accept ID     run the card's acceptance command
//	taskrun verify ID     scope, given-file and gate checks on the task branch
//	taskrun finish ID     verify, move the card to done/ and commit the work
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "taskrun:", err)
		os.Exit(1)
	}
}

type runner struct {
	root     string
	model    string
	attempts int
	out      io.Writer
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("taskrun", flag.ContinueOnError)
	model := fs.String("model", os.Getenv("FROSTMAIL_EXECUTOR_MODEL"), "opencode provider/model for the executor")
	attempts := fs.Int("attempts", 3, "executor attempts before giving up")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root, err := gitOutput(ctx, ".", "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	r := &runner{root: root, model: *model, attempts: *attempts, out: os.Stdout}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New("usage: taskrun [-model M] list|start|run|accept|verify|finish [ID]")
	}
	if rest[0] == "list" {
		return r.list()
	}
	if len(rest) != 2 {
		return fmt.Errorf("%s needs a card ID", rest[0])
	}
	c, err := FindCard(root, rest[1])
	if err != nil {
		return err
	}
	switch rest[0] {
	case "start":
		return r.start(ctx, c)
	case "run":
		return r.runTask(ctx, c)
	case "accept":
		return r.accept(ctx, c, r.out)
	case "verify":
		return r.verify(ctx, c)
	case "finish":
		return r.finish(ctx, c)
	}
	return fmt.Errorf("unknown command %q", rest[0])
}

func (r *runner) list() error {
	cards, err := ListCards(r.root)
	if err != nil {
		return err
	}
	for _, c := range cards {
		fmt.Fprintf(r.out, "%-5s T-%s %s [%s %s]\n", c.State, c.ID, c.Title, c.Milestone, c.Size)
	}
	return nil
}

func branch(c *Card) string { return "task/" + c.ID }

func (r *runner) start(ctx context.Context, c *Card) error {
	if c.State != "todo" {
		return fmt.Errorf("task %s is in %s, not todo", c.ID, c.State)
	}
	if dirty, err := gitOutput(ctx, r.root, "status", "--porcelain"); err != nil || dirty != "" {
		return fmt.Errorf("working tree must be clean to start a task (%v)", err)
	}
	if err := r.git(ctx, "switch", "-c", branch(c)); err != nil {
		return err
	}
	for _, g := range c.Given {
		data, err := os.ReadFile(filepath.Join(r.root, c.GivenSource(g)))
		if err != nil {
			return fmt.Errorf("given file: %w", err)
		}
		dst := filepath.Join(r.root, g)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
	}
	doing := filepath.Join("docs/tasks/doing", filepath.Base(c.Path))
	if err := r.git(ctx, "mv", c.Path, doing); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.root, c.Dir())); err == nil {
		if err := r.git(ctx, "mv", c.Dir(), strings.TrimSuffix(doing, ".md")); err != nil {
			return err
		}
	}
	if err := r.git(ctx, "add", "--", "docs/tasks"); err != nil {
		return err
	}
	if len(c.Given) > 0 {
		if err := r.git(ctx, append([]string{"add", "--"}, c.Given...)...); err != nil {
			return err
		}
	}
	if err := r.git(ctx, "commit", "-q", "-m", fmt.Sprintf("chore(tasks): start T-%s %s", c.ID, c.Title)); err != nil {
		return err
	}
	c.State, c.Path = "doing", doing
	fmt.Fprintf(r.out, "started T-%s on %s\n", c.ID, branch(c))
	return nil
}

func (r *runner) runTask(ctx context.Context, c *Card) error {
	if r.model == "" {
		return errors.New("no executor model: pass -model or set FROSTMAIL_EXECUTOR_MODEL")
	}
	if c.State == "todo" {
		if err := r.start(ctx, c); err != nil {
			return err
		}
	} else if err := r.onBranch(ctx, c); err != nil {
		return err
	}
	card, err := os.ReadFile(filepath.Join(r.root, c.Path))
	if err != nil {
		return err
	}
	prompt := fmt.Sprintf("Implement task card T-%s. Follow docs/tasks/EXECUTOR.md exactly.\n"+
		"When you believe you are done, run `make accept T=%s` and fix any failure.\n\n%s", c.ID, c.ID, card)
	args := []string{"run", "--model", r.model, "--title", "T-" + c.ID, prompt}
	for attempt := 1; attempt <= r.attempts; attempt++ {
		fmt.Fprintf(r.out, "== T-%s executor attempt %d/%d\n", c.ID, attempt, r.attempts)
		if err := r.cmd(ctx, "opencode", args...); err != nil {
			fmt.Fprintf(r.out, "executor exited: %v\n", err)
		}
		var buf bytes.Buffer
		accErr := r.accept(ctx, c, io.MultiWriter(r.out, &buf))
		if accErr == nil {
			buf.Reset()
			verr := r.capture(&buf, func() error { return r.verify(ctx, c) })
			if verr == nil {
				return nil
			}
			args = []string{"run", "--continue", "--model", r.model,
				fmt.Sprintf("`make accept T=%s` passes, but the task does not verify: %v\n"+
					"Fix the cause (run `make ui-fmt` for formatting, then `make check` and, for app cards, `make ui-check`); "+
					"do not edit the given tests.\n\n%s", c.ID, verr, tail(buf.String(), 80))}
			continue
		}
		args = []string{"run", "--continue", "--model", r.model,
			fmt.Sprintf("`make accept T=%s` still fails. Fix the cause; do not edit the given tests.\n\n%s", c.ID, tail(buf.String(), 80))}
	}
	return fmt.Errorf("task %s: still failing after %d attempts; review branch %s", c.ID, r.attempts, branch(c))
}

func (r *runner) accept(ctx context.Context, c *Card, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", c.Acceptance)
	cmd.Dir, cmd.Stdout, cmd.Stderr = r.root, out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("acceptance %q: %w", c.Acceptance, err)
	}
	return nil
}

func (r *runner) onBranch(ctx context.Context, c *Card) error {
	cur, err := gitOutput(ctx, r.root, "branch", "--show-current")
	if err != nil {
		return err
	}
	if cur != branch(c) {
		return fmt.Errorf("task %s is %s; switch to %s first", c.ID, c.State, branch(c))
	}
	return nil
}

// changedSinceStart lists files changed by the executor: committed after the
// start commit, staged, unstaged or untracked.
func (r *runner) changedSinceStart(ctx context.Context, c *Card) ([]string, error) {
	start, err := gitOutput(ctx, r.root, "log", "-1", "--format=%H", "--grep", "^chore(tasks): start T-"+c.ID+" ")
	if err != nil || start == "" {
		return nil, fmt.Errorf("no start commit for T-%s on this branch", c.ID)
	}
	diff, err := gitOutput(ctx, r.root, "diff", "--name-only", start)
	if err != nil {
		return nil, err
	}
	untracked, err := gitOutput(ctx, r.root, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, f := range strings.Fields(diff + "\n" + untracked) {
		if !slices.Contains(files, f) {
			files = append(files, f)
		}
	}
	slices.Sort(files)
	return files, nil
}

func (r *runner) verify(ctx context.Context, c *Card) error {
	if err := r.onBranch(ctx, c); err != nil {
		return err
	}
	changed, err := r.changedSinceStart(ctx, c)
	if err != nil {
		return err
	}
	var problems []string
	if bad := ScopeViolations(c, changed); len(bad) > 0 {
		problems = append(problems, fmt.Sprintf("changed files outside the card's touch list: %v", bad))
	}
	for _, g := range c.Given {
		want, err := os.ReadFile(filepath.Join(r.root, c.GivenSource(g)))
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(r.root, g))
		if err != nil || !bytes.Equal(got, want) {
			problems = append(problems, fmt.Sprintf("given file %s was modified or removed", g))
		}
	}
	if err := r.accept(ctx, c, r.out); err != nil {
		problems = append(problems, err.Error())
	}
	if err := r.cmd(ctx, "make", "check"); err != nil {
		problems = append(problems, "make check: "+err.Error())
	}
	if TouchesApp(c) {
		if err := r.cmd(ctx, "make", "ui-check"); err != nil {
			problems = append(problems, "make ui-check: "+err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("task %s does not verify:\n  %s", c.ID, strings.Join(problems, "\n  "))
	}
	fmt.Fprintf(r.out, "T-%s verifies: %d files changed, all in scope\n", c.ID, len(changed))
	return nil
}

func (r *runner) finish(ctx context.Context, c *Card) error {
	if err := r.verify(ctx, c); err != nil {
		return err
	}
	done := filepath.Join("docs/tasks/done", filepath.Base(c.Path))
	if err := r.git(ctx, "mv", c.Path, done); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(r.root, c.Dir())); err == nil {
		if err := r.git(ctx, "mv", c.Dir(), strings.TrimSuffix(done, ".md")); err != nil {
			return err
		}
	}
	if err := r.git(ctx, append([]string{"add", "--", "docs/tasks"}, c.Touch...)...); err != nil {
		return err
	}
	msg := fmt.Sprintf("feat: %s\n\nTask card T-%s, implemented by the executor model and verified by taskrun.", lowerFirst(c.Title), c.ID)
	if err := r.git(ctx, "commit", "-q", "-m", msg); err != nil {
		return err
	}
	fmt.Fprintf(r.out, "finished T-%s on %s; review and merge it\n", c.ID, branch(c))
	return nil
}

func (r *runner) git(ctx context.Context, args ...string) error { return r.cmd(ctx, "git", args...) }

// capture runs fn with the runner's output also copied into buf.
func (r *runner) capture(buf *bytes.Buffer, fn func() error) error {
	orig := r.out
	r.out = io.MultiWriter(orig, buf)
	defer func() { r.out = orig }()
	return fn()
}

func (r *runner) cmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = r.root, r.out, r.out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, firstArg(args), err)
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", firstArg(args), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func tail(s string, lines int) string {
	parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	return strings.Join(parts, "\n")
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
