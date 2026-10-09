package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// Tasks implements the tasks domain.
func (e *Engine) Tasks() api.TasksService { return tasksService{e.d} }

type tasksService struct{ Deps }

// M4.5's Phase 4 cards implement these (docs/plans/0007).

func (t tasksService) List(context.Context, *api.TasksListParams) ([]api.Task, error) {
	return nil, notBuilt("tasks.list")
}

func (t tasksService) Create(context.Context, *api.TasksCreateParams) (*api.Task, error) {
	return nil, notBuilt("tasks.create")
}

func (t tasksService) Update(context.Context, *api.TasksUpdateParams) (*api.Task, error) {
	return nil, notBuilt("tasks.update")
}

func (t tasksService) Delete(context.Context, *api.TasksDeleteParams) error {
	return notBuilt("tasks.delete")
}
