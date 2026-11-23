package core

import (
	"context"
	"embed"
	"slices"
	"strings"
	"text/template"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core/foundation"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/store"
)

// Stage Guides (Spec 66, ADR 0007; Spec 213, ADR 0057). Code Central
// owns what each Stage, Map Ticket and Prototype must produce and how to
// finish it; a Factory owns how to run the agent. The guides ship with
// the service, like the workflow prose, and name only Code Central
// operations, so any agent on any door can follow them.

// GuideNames are the names with a guide: each Stage, each Map Ticket
// type, prototype, retro, then implementation.
var GuideNames = func() []string {
	var names []string
	for _, s := range domain.Stages() {
		names = append(names, string(s))
	}
	for _, t := range api.WayfinderTypes() {
		names = append(names, api.MapGuidePrefix+string(t))
	}
	return append(names, domain.PrototypeLabel, domain.RetroLabel, api.ImplementationStage)
}()

//go:embed stageguides/*.md
var stageGuides embed.FS

// guideFragments holds the optional lines a Project setting puts into a
// guide at a placeholder. They sit outside stageGuides' glob, so nothing
// mistakes one for a guide.
//
//go:embed stageguides/fragments/*.md
var guideFragments embed.FS

// An optional guide line (ADR 0059) is one named Project setting and one
// fragment: the guide holds a placeholder line, and the fragment's text,
// already a whole bullet, replaces it when the setting is on; when it is
// off, the placeholder line is removed whole.
const (
	subagentExplorationPlaceholder = "<!-- subagent-exploration -->"
	modelAssignmentPlaceholder     = "<!-- model-assignment -->"
	// severalRepositoriesPlaceholder ends the Finishing paragraph of each
	// guide whose Tickets commit. Its Project setting is the Repository
	// list, which the human sets: with more than one Repository the
	// fragment follows as a paragraph of its own, and with the Home
	// Repository alone the line is removed whole.
	severalRepositoriesPlaceholder = "<!-- several-repositories -->"
)

// modelAssignmentFragment is the Model Assignment fragment, a template
// filled from the Project's Reported Models.
var modelAssignmentFragment = template.Must(template.New("model-assignment.md").Funcs(template.FuncMap{
	"list": func(values []string) string {
		quoted := make([]string, len(values))
		for i, v := range values {
			quoted[i] = "`" + v + "`"
		}
		return strings.Join(quoted, ", ")
	},
}).ParseFS(guideFragments, "stageguides/fragments/model-assignment.md"))

func stageGuidesView(g domain.StageGuides) api.StageGuidesView {
	return api.StageGuidesView{SubagentExploration: !g.SubagentExplorationOff, ModelAssignment: g.ModelAssignmentOn}
}

// GetStageGuide is get_stage_guide(stage): the Project's guide named by
// one of GuideNames, as claim would give it now.
func (c *Core) GetStageGuide(_ context.Context, key string, in api.StageGuideInput) (api.StageGuideView, error) {
	p, err := c.projectOf(key)
	if err != nil {
		return api.StageGuideView{}, err
	}
	if !slices.Contains(GuideNames, in.Stage) {
		return api.StageGuideView{}, api.InvalidField("stage", "there is no Stage Guide %q; the guides are %s", in.Stage, strings.Join(GuideNames, ", "))
	}
	return stageGuide(in.Stage, p), nil
}

// projectOf is the stored Project named key, or not_found.
func (c *Core) projectOf(key string) (domain.Project, error) {
	var p domain.Project
	err := c.store.View(func(r store.Reader) error {
		var ok bool
		if p, ok = r.Project(key); !ok {
			return api.UnknownProjectError(key)
		}
		return nil
	})
	return p, err
}

// stageGuide is the named Stage's guide for Project p, ending with the
// trouble rules; the name must be one of GuideNames.
func stageGuide(stage string, p domain.Project) api.StageGuideView {
	g := p.StageGuides
	b, err := stageGuides.ReadFile("stageguides/" + stage + ".md")
	if err != nil {
		panic("core: no Stage Guide for " + stage)
	}
	text := string(b)
	if g.SubagentExplorationOff {
		text = strings.Replace(text, subagentExplorationPlaceholder+"\n", "", 1)
	} else {
		f, err := guideFragments.ReadFile("stageguides/fragments/subagent-exploration.md")
		if err != nil {
			panic("core: no subagent-exploration fragment")
		}
		text = strings.Replace(text, subagentExplorationPlaceholder+"\n", string(f), 1)
	}
	if strings.Contains(text, modelAssignmentPlaceholder) {
		text = strings.Replace(text, modelAssignmentPlaceholder+"\n", modelAssignmentText(p), 1)
	}
	if strings.Contains(text, severalRepositoriesPlaceholder) {
		text = strings.Replace(text, severalRepositoriesPlaceholder+"\n", severalRepositoriesText(p), 1)
	}
	return api.StageGuideView{Stage: stage, Guide: text + "\n" + troubleRules}
}

// severalRepositoriesText is what replaces the several-repositories
// placeholder line: nothing while p has the Home Repository alone, else
// the fragment as a paragraph of its own.
func severalRepositoriesText(p domain.Project) string {
	if len(p.RepositoryList()) < 2 {
		return ""
	}
	f, err := guideFragments.ReadFile("stageguides/fragments/several-repositories.md")
	if err != nil {
		panic("core: no several-repositories fragment")
	}
	return "\n" + string(f)
}

// modelAssignmentText is what replaces the model-assignment placeholder
// line: nothing while Model Assignment is off, else the fragment filled
// from p's Reported Models.
func modelAssignmentText(p domain.Project) string {
	if !p.StageGuides.ModelAssignmentOn {
		return ""
	}
	var b strings.Builder
	data := struct{ Aliases, Levels []string }{p.ReportedModels.Aliases, foundation.ValidEffortLevels(p.ReportedModels)}
	if err := modelAssignmentFragment.Execute(&b, data); err != nil {
		panic("core: model-assignment fragment: " + err.Error())
	}
	return b.String()
}

// SetSubagentExploration is the human's set_subagent_exploration:
// whether the implementation guide has the subagent line. It applies
// from the next Claim and the next get_stage_guide. Like the Project's
// other settings it adds no Move, Edit or Comment; the Project's notice
// announces it.
func (c *Core) SetSubagentExploration(ctx context.Context, key string, in api.SetSubagentExplorationInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_subagent_exploration"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	var before, after api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was := p.StageGuides
		p.StageGuides.SubagentExplorationOff = !in.On
		after = c.projectView(p)
		if p.StageGuides == was {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}

// SetModelAssignment is the human's set_model_assignment: whether the
// tickets Stage sets each Ticket's model and effort. Like the Project's
// other settings it adds no Move, Edit or Comment; the Project's notice
// announces it, and only on a change.
func (c *Core) SetModelAssignment(ctx context.Context, key string, in api.SetModelAssignmentInput) (api.Written[api.ProjectView], error) {
	if err := foundation.HumanOnly(ctx, "set_model_assignment"); err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	var before, after api.ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := foundation.Writable(w, key); err != nil {
			return err
		}
		p, _ := w.Project(key)
		before = c.projectView(p)
		was := p.StageGuides
		p.StageGuides.ModelAssignmentOn = in.On
		after = c.projectView(p)
		if p.StageGuides == was {
			return nil
		}
		return w.PutProject(p)
	})
	if err != nil {
		return api.Written[api.ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}
