package core

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/tomoram/code-central/store"
)

// A Project's Reported Models (Spec code-central-301): the Factory reports
// at startup the effort levels and model aliases its claude accepts, with
// its own --model and --effort flags, with report_models. Code Central
// keeps them, checks effort against the levels, and the Models pickers
// offer them.

// ReportedModelsView is a Project's Reported Models as every Project read
// returns them.
type ReportedModelsView struct {
	// EffortLevels are the valid effort levels: the reported ones, or the
	// seeded ones until a report has given some.
	EffortLevels []string `json:"effort_levels"`
	Aliases      []string `json:"aliases"`
	ModelIDs     []string `json:"model_ids"`
	// ModelFlag and EffortFlag are the Factory's --model and --effort,
	// empty when it reported them unset or hasn't reported.
	ModelFlag  string `json:"model_flag"`
	EffortFlag string `json:"effort_flag"`
	// Reported is when the Factory last reported, left out before any
	// report.
	Reported *time.Time `json:"reported,omitempty"`
}

func reportedModelsView(r store.ReportedModels) ReportedModelsView {
	v := ReportedModelsView{EffortLevels: validLevels(r), Aliases: orEmpty(r.Aliases), ModelIDs: orEmpty(r.ModelIDs), ModelFlag: r.ModelFlag, EffortFlag: r.EffortFlag}
	if !r.Reported.IsZero() {
		at := r.Reported
		v.Reported = &at
	}
	return v
}

// validLevels are the valid effort levels given r: its levels, or the
// seeded ones while it has none.
func validLevels(r store.ReportedModels) []string {
	if len(r.EffortLevels) == 0 {
		return seededEffortLevels
	}
	return r.EffortLevels
}

// ReportModelsInput is the Factory's report_models(effort_levels,
// aliases, model_flag, effort_flag, full?).
type ReportModelsInput struct {
	// EffortLevels and Aliases are what the Factory read from claude
	// --help; left empty, the kept ones stay.
	EffortLevels []string `json:"effort_levels"`
	Aliases      []string `json:"aliases"`
	// ModelFlag and EffortFlag are the Factory's own flags, empty when
	// unset; they always replace the kept ones.
	ModelFlag  string `json:"model_flag"`
	EffortFlag string `json:"effort_flag"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// ReportModels is the Factory's report_models, made when it starts. Empty
// levels or aliases leave the kept ones, so a failed read of claude
// --help loses nothing; the flags are always replaced; model IDs are never
// removed. It records when the report arrived.
func (c *Core) ReportModels(_ context.Context, key string, in ReportModelsInput) (Written[ProjectView], error) {
	levels, err := reportedList("effort_levels", in.EffortLevels)
	if err != nil {
		return Written[ProjectView]{}, err
	}
	aliases, err := reportedList("aliases", in.Aliases)
	if err != nil {
		return Written[ProjectView]{}, err
	}
	modelFlag, effortFlag := strings.TrimSpace(in.ModelFlag), strings.TrimSpace(in.EffortFlag)
	for _, f := range []struct{ field, v string }{{"model_flag", modelFlag}, {"effort_flag", effortFlag}} {
		if strings.ContainsAny(f.v, "\r\n") {
			return Written[ProjectView]{}, invalid(f.field, "a flag's value is one line")
		}
	}
	var before, after ProjectView
	err = c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		r := p.ReportedModels
		if len(levels) > 0 {
			r.EffortLevels = levels
		}
		if len(aliases) > 0 {
			r.Aliases = aliases
		}
		r.ModelFlag, r.EffortFlag, r.Reported = modelFlag, effortFlag, c.now()
		p.ReportedModels = r
		after = c.projectView(p)
		return w.PutProject(p)
	})
	if err != nil {
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// reportedList is vs trimmed, without blanks or repeats, or why one of
// them, in field, isn't a single line.
func reportedList(field string, vs []string) ([]string, error) {
	var out []string
	for _, v := range vs {
		v = strings.TrimSpace(v)
		switch {
		case strings.ContainsAny(v, "\r\n"):
			return nil, invalid(field, "%q spans more than one line; each is one line", v)
		case v != "" && !slices.Contains(out, v):
			out = append(out, v)
		}
	}
	return out, nil
}
