package core

import (
	"context"
	"embed"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Stage Guides (Spec 66, ADR 0007; Spec 213, ADR 0016). Code Central
// owns what each Stage, Map Ticket and Prototype must produce and how to
// finish it; a Factory owns how to run the agent. The guides ship with
// the service, like the workflow prose, and name only Code Central
// operations, so any agent on any door can follow them.

// ImplementationStage names the Stage Guide for a Ticket with no
// reserved type Label.
const ImplementationStage = "implementation"

// mapGuidePrefix starts the Guide name of each Map Ticket type, which
// keeps the Map's grilling apart from the grilling Stage.
const mapGuidePrefix = "map-"

// GuideNames are the names with a guide: each Stage, each Map Ticket
// type, prototype, then implementation.
var GuideNames = func() []string {
	names := slices.Clone(Stages)
	for _, t := range WayfinderTypes {
		names = append(names, mapGuidePrefix+t)
	}
	return append(names, PrototypeLabel, ImplementationStage)
}()

//go:embed stageguides/*.md
var stageGuides embed.FS

// guideFragments holds the optional lines a Project setting puts into a
// guide at a placeholder. They sit outside stageGuides' glob, so nothing
// mistakes one for a guide.
//
//go:embed stageguides/fragments/*.md
var guideFragments embed.FS

// An optional guide line (ADR 0034) is one named Project setting and one
// fragment: the guide holds a placeholder line, and the fragment's text,
// already a whole bullet, replaces it when the setting is on; when it is
// off, the placeholder line is removed whole.
const subagentExplorationPlaceholder = "<!-- subagent-exploration -->"

// StageGuidesView is a Project's Stage Guides settings as every Project
// read returns them.
type StageGuidesView struct {
	// SubagentExploration is whether the implementation guide tells the
	// agent to send reading across several files to a subagent.
	SubagentExploration bool `json:"subagent_exploration"`
}

func stageGuidesView(g store.StageGuides) StageGuidesView {
	return StageGuidesView{SubagentExploration: !g.SubagentExplorationOff}
}

// StageGuideInput is get_stage_guide's input.
type StageGuideInput struct {
	Stage string `json:"stage"`
}

// StageGuideView is one Stage Guide: get_stage_guide's result, and the
// guide in claim's.
type StageGuideView struct {
	Stage string `json:"stage"`
	Guide string `json:"guide"`
}

// GetStageGuide is get_stage_guide(stage): the Project's guide named by
// one of GuideNames, as claim would give it now.
func (c *Core) GetStageGuide(_ context.Context, key string, in StageGuideInput) (StageGuideView, error) {
	p, err := c.projectOf(key)
	if err != nil {
		return StageGuideView{}, err
	}
	if !slices.Contains(GuideNames, in.Stage) {
		return StageGuideView{}, invalid("stage", "there is no Stage Guide %q; the guides are %s", in.Stage, strings.Join(GuideNames, ", "))
	}
	return stageGuide(in.Stage, p.StageGuides), nil
}

// projectOf is the stored Project named key, or not_found.
func (c *Core) projectOf(key string) (store.Project, error) {
	var p store.Project
	err := c.store.View(func(r store.Reader) error {
		var ok bool
		if p, ok = r.Project(key); !ok {
			return unknownProject(key)
		}
		return nil
	})
	return p, err
}

// stageGuide is the named Stage's guide for a Project with settings g,
// ending with the trouble rules; the name must be one of GuideNames.
func stageGuide(stage string, g store.StageGuides) StageGuideView {
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
	return StageGuideView{Stage: stage, Guide: text + "\n" + troubleRules}
}

// GuideName names the guide for a Ticket with these Labels: its Stage,
// map-<type> for a Map Ticket, prototype for a Prototype, or
// ImplementationStage only when it has no reserved type Label.
func GuideName(labels []string) string {
	for _, l := range labels {
		if g, ok := typeGuide(l); ok {
			return g
		}
	}
	return ImplementationStage
}

// SetSubagentExplorationInput is the human's
// set_subagent_exploration(on, full?).
type SetSubagentExplorationInput struct {
	On bool `json:"on"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}

// SetSubagentExploration is the human's set_subagent_exploration:
// whether the implementation guide has the subagent line. It applies
// from the next Claim and the next get_stage_guide. Like the Project's
// other settings it adds no Move, Edit or Comment; the Project's notice
// announces it.
func (c *Core) SetSubagentExploration(ctx context.Context, key string, in SetSubagentExplorationInput) (Written[ProjectView], error) {
	if err := humanOnly(ctx, "set_subagent_exploration"); err != nil {
		return Written[ProjectView]{}, err
	}
	var before, after ProjectView
	err := c.store.Update(func(w store.Writer) error {
		if err := writable(w, key); err != nil {
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
		return Written[ProjectView]{}, err
	}
	return acknowledgeProject(before, after, in.Full, c.now())
}
