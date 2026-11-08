package core

import (
	"context"
	"embed"
	"slices"
	"strings"

	"github.com/tomoram/code-central/store"
)

// Stage Guides (Spec code-central-66, ADR 0007). Code Central owns what
// each Stage must produce and how to finish it; a Factory owns how to run
// the agent. The guides ship with the service, like the workflow prose,
// and name only Code Central operations, so any agent on any door can
// follow them.

// ImplementationStage names the Stage Guide for a Ticket with no Stage
// Label.
const ImplementationStage = "implementation"

// GuideStages are the Stages with a guide: each Stage, then implementation.
var GuideStages = append(slices.Clone(Stages), ImplementationStage)

//go:embed stageguides/*.md
var stageGuides embed.FS

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

// GetStageGuide is get_stage_guide(stage): the guide for grilling, spec,
// tickets or implementation. It is the same for every Project.
func (c *Core) GetStageGuide(_ context.Context, in StageGuideInput) (StageGuideView, error) {
	if !slices.Contains(GuideStages, in.Stage) {
		return StageGuideView{}, invalid("stage", "there is no Stage Guide %q; the Stages are %s", in.Stage, strings.Join(GuideStages, ", "))
	}
	return stageGuide(in.Stage), nil
}

// stageGuide is the named Stage's guide; the name must be one of GuideStages.
func stageGuide(stage string) StageGuideView {
	b, err := stageGuides.ReadFile("stageguides/" + stage + ".md")
	if err != nil {
		panic("core: no Stage Guide for " + stage)
	}
	return StageGuideView{Stage: stage, Guide: string(b)}
}

// guideStage names the guide for a Ticket: its Stage, or implementation
// when it has no Stage Label.
func guideStage(t store.Ticket) string {
	if s := stageOf(t); s != "" {
		return s
	}
	return ImplementationStage
}
