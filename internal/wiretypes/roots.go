package main

import (
	"reflect"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
)

// roots are the response types the UI reads, each written with every
// type it reaches. An operation that returns a new top-level view adds
// its type here. submit_requirement's and submit_map's results aren't
// roots: their Written values encode themselves, as an Acknowledgement
// or the record, so the UI types them over the acknowledgements.
var roots = []reflect.Type{
	reflect.TypeFor[core.ProjectView](),
	reflect.TypeFor[core.ProjectRow](),
	reflect.TypeFor[core.TargetView](),
	reflect.TypeFor[core.LinkedRow](),
	reflect.TypeFor[core.TicketView](),
	reflect.TypeFor[core.TicketRow](),
	reflect.TypeFor[core.SpecDetail](),
	reflect.TypeFor[core.SpecRow](),
	reflect.TypeFor[core.SpecQuestions](),
	reflect.TypeFor[core.NeedsMeView](),
	reflect.TypeFor[core.TranscriptView](),
	reflect.TypeFor[core.CIOverview](),
	reflect.TypeFor[core.UsageView](),
	reflect.TypeFor[core.SessionUsageView](),
	reflect.TypeFor[core.ConversationView](),
	reflect.TypeFor[core.OptionsView](),
	reflect.TypeFor[core.Acknowledgement](),
}

// fields are the record views a write acknowledges, each written as an
// array of its JSON names, which the UI narrows changed to.
var fields = []Fields{
	fieldsOf[core.TicketView]("TicketFields"),
	fieldsOf[core.SpecView]("SpecFields"),
}

// enums are the value lists of the typed strings the roots reach, each
// written as a union with its value array, in the list's order.
var enums = []Enum{
	enumOf("States", domain.States),
	enumOf("NeedsMeReasons", core.NeedsMeReasons),
	enumOf("TriageActions", core.TriageActions()),
	enumOf("Modes", domain.Modes),
	enumOf("LandingMethods", domain.LandingMethods),
	enumOf("RepositoryLandings", core.RepositoryLandings()),
	enumOf("CodeHosts", core.CodeHosts()),
	enumOf("CloneStates", core.CloneStates()),
	enumOf("CIProviders", domain.CIProviders),
	enumOf("AzureBoardsModes", domain.AzureBoardsModes),
	enumOf("WatcherStates", core.WatcherStates()),
	enumOf("WatcherProblems", core.WatcherProblems()),
	enumOf("AutoAcceptKinds", domain.AutoAcceptKinds),
	enumOf("AutoAcceptCauses", domain.AutoAcceptCauses),
	enumOf("ChoiceSources", core.ChoiceSources()),
	enumOf("CIResults", core.CIResults()),
	enumOf("CIRunStatuses", domain.CIRunStatuses),
	enumOf("CIConclusions", domain.CIConclusions),
	enumOf("PullRequestStates", domain.PullRequestStates),
	enumOf("FactoryStatusNames", core.FactoryStatusNames()),
	enumOf("BaseStates", core.BaseStates()),
	enumOf("RecordKinds", core.RecordKinds()),
	enumOf("ExternalStates", domain.ExternalStates),
	enumOf("CIFailureOptions", core.CIFailureOptions()),
	enumOf("CIUnavailableCauses", core.CIUnavailableCauses()),
	enumOf("Stages", core.Stages()),
	enumOf("SegmentStages", core.SegmentStages()),
	enumOf("SegmentProgresses", core.SegmentProgresses()),
	enumOf("NodeTypes", core.NodeTypes),
	enumOf("SpecKinds", core.SpecKinds()),
	enumOf("Recommendations", domain.Recommendations),
	enumOf("QuestionStatuses", domain.QuestionStatuses),
	enumOf("SessionEventKinds", core.SessionEventKinds()),
	enumOf("FocusClears", domain.FocusClears),
	enumOf("FocusPopups", domain.FocusPopups),
	enumOf("LimitsPopups", domain.LimitsPopups),
	enumOf("ChatAvailabilities", core.ChatAvailabilities()),
	enumOf("ChatAuthors", core.ChatAuthors()),
	enumOf("ChatOutcomes", core.ChatOutcomes()),
}
