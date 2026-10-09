package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// People implements the people domain.
func (e *Engine) People() api.PeopleService { return people{e.d} }

type people struct{ Deps }

// Task T-0062 implements List, Get, Card and Photo; the planner, Add.

func (p people) List(context.Context, *api.PeopleListParams) ([]api.PersonSummary, error) {
	return nil, notBuilt("people.list")
}

func (p people) Get(context.Context, *api.PeopleGetParams) (*api.Person, error) {
	return nil, notBuilt("people.get")
}

func (p people) Card(context.Context, *api.PeopleCardParams) (*api.ContactCard, error) {
	return nil, notBuilt("people.card")
}

func (p people) Photo(context.Context, *api.PeoplePhotoParams) (*api.Photo, error) {
	return nil, notBuilt("people.photo")
}

func (p people) Add(context.Context, *api.PeopleAddParams) (*api.Person, error) {
	return nil, notBuilt("people.add")
}
