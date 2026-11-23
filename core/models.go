package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// A Project's Models (Spec code-central-301): the human sets a model and
// effort for the Project default and for each kind of work with
// set_models. They are tiers 2 and 3 of the Model Choice, below a
// Ticket's Tags and above the Factory's flags.

func modelsView(m domain.Models) api.ModelsView {
	v := api.ModelsView{Default: api.ModelRowView(m.Default), Chat: api.ModelRowView(m.Chat), Kinds: []api.KindModelsView{}}
	for _, g := range GuideNames {
		v.Kinds = append(v.Kinds, api.KindModelsView{Kind: g, ModelRowView: api.ModelRowView(m.Kind(g))})
	}
	return v
}

// SetModels is the human's set_models: it replaces the Project's Models
// rows whole. Like the Project's other settings it adds no Move, Edit or
// Comment; the Project's notice announces it.
func (c *Core) SetModels(ctx context.Context, key string, in api.SetModelsInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_models"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	var before, after api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		m, err := checkedModels(w, key, in)
		if err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was := p.Models
		p.Models = m
		after = c.projectView(p)
		if m.Equal(was) {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// checkedModels checks in's rows and returns them as stored: only the rows that
// set something, in GuideNames order.
func checkedModels(r store.Reader, key string, in api.SetModelsInput) (domain.Models, error) {
	var m domain.Models
	var err error
	if m.Default, err = modelRow(r, key, "default", "the Project default", in.Default); err != nil {
		return domain.Models{}, err
	}
	if m.Chat, err = modelRow(r, key, "chat", "the Chat row", in.Chat); err != nil {
		return domain.Models{}, err
	}
	seen := map[string]bool{}
	for _, k := range in.Kinds {
		switch {
		case !slices.Contains(GuideNames, k.Kind):
			return domain.Models{}, api.InvalidField("kinds", "%q isn't a kind of work; the kinds are %s", k.Kind, strings.Join(GuideNames, ", "))
		case seen[k.Kind]:
			return domain.Models{}, api.InvalidField("kinds", "there are two rows for %q; give each kind of work one", k.Kind)
		}
		seen[k.Kind] = true
		row, err := modelRow(r, key, "kinds."+k.Kind, "the "+k.Kind+" row", k.ModelRowView)
		if err != nil {
			return domain.Models{}, err
		}
		if row != (domain.ModelRow{}) {
			m.Kinds = append(m.Kinds, domain.KindModels{Kind: k.Kind, ModelRow: row})
		}
	}
	slices.SortFunc(m.Kinds, func(a, b domain.KindModels) int {
		return slices.Index(GuideNames, a.Kind) - slices.Index(GuideNames, b.Kind)
	})
	return m, nil
}

// modelRow checks one row, named for messages, whose fields start with
// field: a model is one line that isn't blank, an effort one of the
// Project's valid levels, and either may be empty to leave it unset.
func modelRow(r store.Reader, key, field, name string, v api.ModelRowView) (domain.ModelRow, error) {
	model := strings.TrimSpace(v.Model)
	switch {
	case v.Model != "" && model == "":
		return domain.ModelRow{}, api.InvalidField(field+".model", "%s's model is blank; leave it empty for Default", name)
	case strings.ContainsAny(model, "\r\n"):
		return domain.ModelRow{}, api.InvalidField(field+".model", "%s's model spans more than one line; a model is one line", name)
	}
	if v.Effort != "" {
		if err := foundation.CheckEffort(r, key, field+".effort", v.Effort); err != nil {
			return domain.ModelRow{}, err
		}
	}
	return domain.ModelRow{Model: model, Effort: v.Effort}, nil
}
