package lodge_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// TestTheEngineKeepsAnAgentsTaskAndReminderWritesInScope: ;write admits
// tasks.create, tasks.update and message.remind for the scope's accounts
// alone, and a scope without it admits none (ADR-0042).
func TestTheEngineKeepsAnAgentsTaskAndReminderWritesInScope(t *testing.T) {
	p := seedPIM(t)
	ctx := t.Context()
	read, _ := p.srv.DialAs(t, agent("account="+itoa(p.a)))
	for method, params := range map[string]any{
		"tasks.create":   map[string]any{"title": "x"},
		"tasks.update":   map[string]any{"id": p.tasks["Call Iris"], "completed": true},
		"message.remind": map[string]any{"ids": []int64{p.msgA}},
	} {
		if err := read.Call(ctx, method, params, nil); !errors.Is(err, api.ErrMethodNotFound) {
			t.Errorf("%s without ;write = %v, want methodNotFound", method, err)
		}
	}

	// Under account=B, a new task goes to B's list, though A comes first.
	w, _ := p.srv.DialAs(t, agent("account="+itoa(p.b)+";write"))
	task, err := w.Tasks().Create(ctx, &api.TasksCreateParams{Title: "Call Iris"})
	if err != nil || task.ListID != p.listB || task.AccountID != p.b {
		t.Errorf("tasks.create without a list under account=%d = %+v, %v; want that account's list", p.b, task, err)
	}
	if task, err := w.Tasks().Update(ctx, &api.TasksUpdateParams{ID: p.tasks["B task"], Completed: new(true)}); err != nil || !task.Completed {
		t.Errorf("tasks.update in scope = %+v, %v", task, err)
	}
	if err := w.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{p.msgB}, At: new(pimNow.Add(time.Hour))}); err != nil {
		t.Errorf("message.remind in scope: %v", err)
	}
	notFound := map[string]error{
		"tasks.create in the other account's list": func() error {
			_, err := w.Tasks().Create(ctx, &api.TasksCreateParams{Title: "x", ListID: &p.listA})
			return err
		}(),
		"tasks.create under the other account's task": func() error {
			_, err := w.Tasks().Create(ctx, &api.TasksCreateParams{Title: "x", ListID: &p.listB, ParentID: new(p.tasks["Call Iris"])})
			return err
		}(),
		"tasks.update of the other account's": func() error {
			_, err := w.Tasks().Update(ctx, &api.TasksUpdateParams{ID: p.tasks["Call Iris"], Completed: new(true)})
			return err
		}(),
		"message.remind of the other account's": w.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{p.msgB, p.msgA},
			At: new(pimNow.Add(time.Hour))}),
	}
	for what, err := range notFound {
		if !errors.Is(err, api.ErrNotFound) {
			t.Errorf("%s = %v, want notFound", what, err)
		}
	}
	local := p.srv.Dial(t)
	if iris, err := local.Tasks().Get(ctx, &api.TasksGetParams{ID: p.tasks["Call Iris"]}); err != nil || iris.Completed {
		t.Errorf("account A's task after the agent's update = %+v, %v; want it open", iris, err)
	}
	if sums, err := local.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{p.msgA}}); err != nil || sums[0].RemindAt != nil {
		t.Errorf("account A's message after the agent's reminder = %+v, %v; want none", sums, err)
	}
}

// TestAgentsWriteTasksAndRemindersThroughLodge: create_task,
// complete_task and remind_me under account=<id>;write, and their limits.
func TestAgentsWriteTasksAndRemindersThroughLodge(t *testing.T) {
	p := seedPIM(t)
	ctx := t.Context()
	l := lodgeAt(t, p.srv)
	reader, err := mcpClient(t, l, "claude-read", "account="+itoa(p.a))
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"create_task", "complete_task", "remind_me"} {
		if slices.Contains(toolNames(t, reader), tool) {
			t.Errorf("%s offered without ;write", tool)
		}
	}
	cs, err := mcpClient(t, l, "claude", "account="+itoa(p.a)+";write")
	if err != nil {
		t.Fatal(err)
	}
	local := p.srv.Dial(t)

	var made struct {
		TaskID    int64  `json:"task_id"`
		ListID    int64  `json:"list_id"`
		List      string `json:"list"`
		AccountID int64  `json:"account_id"`
		Title     string `json:"title"`
		Due       string `json:"due"`
		Completed bool   `json:"completed"`
		Note      string `json:"note"`
	}
	if call(t, cs, "create_task", map[string]any{"title": "Call Iris back", "due": "2026-10-23", "notes": "About Saturday"}, &made) ||
		made.ListID != p.listA || made.List != "Tasks A" || made.AccountID != p.a || made.Due != "2026-10-23" || made.Note == "" {
		t.Fatalf("create_task = %+v, want it in Tasks A, due Friday", made)
	}
	task, err := local.Tasks().Get(ctx, &api.TasksGetParams{ID: made.TaskID})
	if err != nil || task.Title != "Call Iris back" || task.Notes != "About Saturday" || task.Due != "2026-10-23" {
		t.Errorf("the task created = %+v, %v", task, err)
	}
	var listed struct {
		Tasks []struct {
			ID int64 `json:"id"`
		} `json:"tasks"`
	}
	if call(t, cs, "list_tasks", map[string]any{"list_id": p.listA}, &listed) ||
		!slices.ContainsFunc(listed.Tasks, func(x struct {
			ID int64 `json:"id"`
		}) bool {
			return x.ID == made.TaskID
		}) {
		t.Errorf("list_tasks after create_task = %+v, want task %d", listed, made.TaskID)
	}
	if call(t, cs, "complete_task", map[string]any{"task_id": made.TaskID}, &made) || !made.Completed {
		t.Errorf("complete_task = %+v, want it completed", made)
	}
	if task, _ := local.Tasks().Get(ctx, &api.TasksGetParams{ID: made.TaskID}); !task.Completed {
		t.Errorf("the task after complete_task = %+v", task)
	}
	if call(t, cs, "complete_task", map[string]any{"task_id": made.TaskID, "completed": false}, &made) || made.Completed {
		t.Errorf("complete_task completed false = %+v, want it open", made)
	}

	var reminded struct {
		MessageID int64  `json:"message_id"`
		At        string `json:"at"`
	}
	at := pimNow.Add(48 * time.Hour).In(time.FixedZone("CEST", 2*3600)).Format(time.RFC3339)
	if call(t, cs, "remind_me", map[string]any{"message_id": p.msgA, "at": at}, &reminded) || reminded.At != at {
		t.Errorf("remind_me = %+v, want at %s", reminded, at)
	}
	sums, err := local.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{p.msgA}})
	if err != nil || sums[0].RemindAt == nil || !sums[0].RemindAt.Equal(pimNow.Add(48*time.Hour)) {
		t.Errorf("message A after remind_me = %+v, %v", sums, err)
	}
	reminded.At = ""
	if call(t, cs, "remind_me", map[string]any{"message_id": p.msgA}, &reminded) || reminded.At != "" {
		t.Errorf("remind_me without at = %+v, want it cleared", reminded)
	}
	if sums, _ := local.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{p.msgA}}); sums[0].RemindAt != nil {
		t.Errorf("message A after clearing = %+v", sums[0])
	}

	for tool, args := range map[string]map[string]any{
		"create_task in the other account's list": {"title": "x", "list_id": p.listB},
		"complete_task of the other account's":    {"task_id": p.tasks["B task"]},
		"remind_me of the other account's":        {"message_id": p.msgB, "at": at},
	} {
		name, _, _ := strings.Cut(tool, " ")
		if !call(t, cs, name, args, &made) {
			t.Errorf("%s answered no error", tool)
		}
	}

	// The limits, at their values and one past.
	for _, title := range []string{"x", strings.Repeat("t", 1024)} {
		if call(t, cs, "create_task", map[string]any{"title": title}, &made) {
			t.Errorf("create_task with a %d-byte title answered an error", len(title))
		}
	}
	if call(t, cs, "create_task", map[string]any{"title": "Notes", "notes": strings.Repeat("n", 32<<10)}, &made) {
		t.Error("create_task with 32 KiB of notes answered an error")
	}
	for _, args := range []map[string]any{
		{"title": ""}, {"title": "  "}, {"title": strings.Repeat("t", 1025)},
		{"title": "Notes", "notes": strings.Repeat("n", 32<<10+1)}, {"title": "x", "due": "Friday"},
	} {
		if !call(t, cs, "create_task", args, &made) {
			t.Errorf("create_task with %d-byte title, %d-byte notes, due %v answered no error",
				len(args["title"].(string)), len(stringArg(args, "notes")), args["due"])
		}
	}
	if call(t, cs, "remind_me", map[string]any{"message_id": p.msgA, "at": pimNow.Add(365 * 24 * time.Hour).Format(time.RFC3339)}, &reminded) {
		t.Error("remind_me 365 days ahead answered an error")
	}
	for _, at := range []string{
		pimNow.Add(365*24*time.Hour + time.Second).Format(time.RFC3339),
		pimNow.Add(-time.Second).Format(time.RFC3339),
		pimNow.Format(time.RFC3339),
		"2026-10-21T09:00:00",
	} {
		if !call(t, cs, "remind_me", map[string]any{"message_id": p.msgA, "at": at}, &reminded) {
			t.Errorf("remind_me at %s answered no error", at)
		}
	}
}

func stringArg(args map[string]any, key string) string {
	s, _ := args[key].(string)
	return s
}
