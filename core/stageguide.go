package core

import (
	"context"
	"embed"
	"slices"
	"strings"
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

// GetStageGuide is get_stage_guide(stage): the guide named by one of
// GuideNames. It is the same for every Project.
func (c *Core) GetStageGuide(_ context.Context, in StageGuideInput) (StageGuideView, error) {
	if !slices.Contains(GuideNames, in.Stage) {
		return StageGuideView{}, invalid("stage", "there is no Stage Guide %q; the guides are %s", in.Stage, strings.Join(GuideNames, ", "))
	}
	return stageGuide(in.Stage), nil
}

// stageGuide is the named Stage's guide, ending with the trouble rules;
// the name must be one of GuideNames.
func stageGuide(stage string) StageGuideView {
	b, err := stageGuides.ReadFile("stageguides/" + stage + ".md")
	if err != nil {
		panic("core: no Stage Guide for " + stage)
	}
	return StageGuideView{Stage: stage, Guide: string(b) + "\n" + troubleRules}
}

// GuideName names the guide for a Ticket with these Labels: its Stage,
// map-<type> for a Map Ticket, prototype for a Prototype, or
// ImplementationStage only when it has no reserved type Label.
func GuideName(labels []string) string {
	if s := StageOf(labels); s != "" {
		return s
	}
	for _, l := range labels {
		if l == PrototypeLabel {
			return PrototypeLabel
		}
		if t, ok := strings.CutPrefix(l, WayfinderPrefix); ok && slices.Contains(WayfinderTypes, t) {
			return mapGuidePrefix + t
		}
	}
	return ImplementationStage
}
