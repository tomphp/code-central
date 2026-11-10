package core

import (
	"context"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// A Project's Models (Spec code-central-301): the human sets a model and
// effort for the Project default and for each kind of work with
// set_models. They are tiers 2 and 3 of the Model Choice, below a
// Ticket's Tags and above the Factory's flags.

// ModelsView is a Project's Models as every Project read returns them:
// the Project default row, then one row for every kind of work, in
// GuideNames order, each empty where it sets nothing.
type ModelsView struct {
	Default ModelRowView     `json:"default"`
	Kinds   []KindModelsView `json:"kinds"`
}

// ModelRowView is one Models row's model and effort, each empty when the
// row leaves it to the tier below.
type ModelRowView struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// KindModelsView is the Models row for one kind of work, a guide name.
type KindModelsView struct {
	Kind string `json:"kind"`
	ModelRowView
}

func modelsView(m store.Models) ModelsView {
	v := ModelsView{Default: ModelRowView(m.Default), Kinds: []KindModelsView{}}
	for _, g := range GuideNames {
		v.Kinds = append(v.Kinds, KindModelsView{Kind: g, ModelRowView: ModelRowView(m.Kind(g))})
	}
	return v
}

// SetModelsInput is the human's set_models(default, kinds, full?).
type SetModelsInput struct {
	Default ModelRowView `json:"default"`
	// Kinds are the rows for kinds of work; a kind left out, or a row
	// setting nothing, sets nothing.
	Kinds []KindModelsView `json:"kinds"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetModels is the human's set_models: it replaces the Project's Models
// rows whole. Like the Project's other settings it adds no Move, Edit or
// Comment; the Project's notice announces it.
func (c *Core) SetModels(ctx context.Context, key string, in SetModelsInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_models"); err != nil {
		return Written[ProjectView]{}, err
	}
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		m, err := models(w, key, in)
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
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// models checks in's rows and returns them as stored: only the rows that
// set something, in GuideNames order.
func models(r store.Reader, key string, in SetModelsInput) (store.Models, error) {
	var m store.Models
	var err error
	if m.Default, err = modelRow(r, key, "default", "the Project default", in.Default); err != nil {
		return store.Models{}, err
	}
	seen := map[string]bool{}
	for _, k := range in.Kinds {
		switch {
		case !slices.Contains(GuideNames, k.Kind):
			return store.Models{}, invalid("kinds", "%q isn't a kind of work; the kinds are %s", k.Kind, strings.Join(GuideNames, ", "))
		case seen[k.Kind]:
			return store.Models{}, invalid("kinds", "there are two rows for %q; give each kind of work one", k.Kind)
		}
		seen[k.Kind] = true
		row, err := modelRow(r, key, "kinds."+k.Kind, "the "+k.Kind+" row", k.ModelRowView)
		if err != nil {
			return store.Models{}, err
		}
		if row != (store.ModelRow{}) {
			m.Kinds = append(m.Kinds, store.KindModels{Kind: k.Kind, ModelRow: row})
		}
	}
	slices.SortFunc(m.Kinds, func(a, b store.KindModels) int {
		return slices.Index(GuideNames, a.Kind) - slices.Index(GuideNames, b.Kind)
	})
	return m, nil
}

// modelRow checks one row, named for messages, whose fields start with
// field: a model is one line that isn't blank, an effort one of the
// Project's valid levels, and either may be empty to leave it unset.
func modelRow(r store.Reader, key, field, name string, v ModelRowView) (store.ModelRow, error) {
	model := strings.TrimSpace(v.Model)
	switch {
	case v.Model != "" && model == "":
		return store.ModelRow{}, invalid(field+".model", "%s's model is blank; leave it empty for Default", name)
	case strings.ContainsAny(model, "\r\n"):
		return store.ModelRow{}, invalid(field+".model", "%s's model spans more than one line; a model is one line", name)
	}
	if v.Effort != "" {
		if err := checkEffort(r, key, field+".effort", v.Effort); err != nil {
			return store.ModelRow{}, err
		}
	}
	return store.ModelRow{Model: model, Effort: v.Effort}, nil
}
