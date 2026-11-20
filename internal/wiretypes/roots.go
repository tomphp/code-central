package main

import (
	"reflect"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// roots are the response types the UI reads, each written with every
// type it reaches. An operation that returns a new top-level view adds
// its type here. submit_requirement's and submit_map's results aren't
// roots: their Written values encode themselves, as an Acknowledgement
// or the record, so the UI types them over the acknowledgements.
var roots = []reflect.Type{
	reflect.TypeFor[api.ProjectView](),
	reflect.TypeFor[api.ProjectRow](),
	reflect.TypeFor[api.TargetView](),
	reflect.TypeFor[api.LinkedRow](),
	reflect.TypeFor[api.TicketView](),
	reflect.TypeFor[api.TicketRow](),
	reflect.TypeFor[api.SpecDetail](),
	reflect.TypeFor[api.SpecRow](),
	reflect.TypeFor[api.SpecQuestions](),
	reflect.TypeFor[api.NeedsMeView](),
	reflect.TypeFor[api.TranscriptView](),
	reflect.TypeFor[api.CIOverview](),
	reflect.TypeFor[api.UsageView](),
	reflect.TypeFor[api.SessionUsageView](),
	reflect.TypeFor[api.StorageView](),
	reflect.TypeFor[api.ConversationView](),
	reflect.TypeFor[api.OptionsView](),
	reflect.TypeFor[api.Acknowledgement](),
}

// fields are the record views a write acknowledges, each written as an
// array of its JSON names, which the UI narrows changed to.
var fields = []Fields{
	fieldsOf[api.TicketView]("TicketFields"),
	fieldsOf[api.SpecView]("SpecFields"),
}

// enums are the value lists of the typed strings the roots reach, each
// written as a union with its value array, in the list's order.
var enums = []Enum{
	enumOf("States", domain.States),
	enumOf("NeedsMeReasons", api.NeedsMeReasons),
	enumOf("TriageActions", api.TriageActions()),
	enumOf("Modes", domain.Modes),
	enumOf("LandingMethods", domain.LandingMethods),
	enumOf("RepositoryLandings", api.RepositoryLandings()),
	enumOf("CodeHosts", api.CodeHosts()),
	enumOf("CloneStates", api.CloneStates()),
	enumOf("CIProviders", domain.CIProviders),
	enumOf("AzureBoardsModes", domain.AzureBoardsModes),
	enumOf("WatcherStates", api.WatcherStates()),
	enumOf("WatcherProblems", api.WatcherProblems()),
	enumOf("AutoAcceptKinds", domain.AutoAcceptKinds),
	enumOf("AutoAcceptCauses", domain.AutoAcceptCauses),
	enumOf("ChoiceSources", api.ChoiceSources()),
	enumOf("CIResults", api.CIResults()),
	enumOf("CIRunStatuses", domain.CIRunStatuses),
	enumOf("CIConclusions", domain.CIConclusions),
	enumOf("PullRequestStates", domain.PullRequestStates),
	enumOf("FactoryStatusNames", api.FactoryStatusNames()),
	enumOf("BaseStates", api.BaseStates()),
	enumOf("RecordKinds", api.RecordKinds()),
	enumOf("ExternalStates", domain.ExternalStates),
	enumOf("CIFailureOptions", api.CIFailureOptions()),
	enumOf("ToMergeOptions", api.ToMergeOptions()),
	enumOf("CIUnavailableCauses", api.CIUnavailableCauses()),
	enumOf("Stages", domain.Stages()),
	enumOf("SegmentStages", api.SegmentStages()),
	enumOf("SegmentProgresses", api.SegmentProgresses()),
	enumOf("NodeTypes", api.NodeTypes),
	enumOf("SpecKinds", api.SpecKinds()),
	enumOf("Recommendations", domain.Recommendations),
	enumOf("QuestionStatuses", domain.QuestionStatuses),
	enumOf("SessionEventKinds", api.SessionEventKinds()),
	enumOf("FocusClears", domain.FocusClears),
	enumOf("FocusPopups", domain.FocusPopups),
	enumOf("LimitsPopups", domain.LimitsPopups),
	enumOf("ChatAvailabilities", api.ChatAvailabilities()),
	enumOf("ChatAuthors", api.ChatAuthors()),
	enumOf("ChatOutcomes", api.ChatOutcomes()),
}
