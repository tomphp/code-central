package ops

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// Submitting work and the Stage Guides. The submits write as human
// whatever Actor the request names; core sees to that.

// SubmitRequirement is submit_requirement.
var SubmitRequirement = &Definition[api.SubmitRequirementInput, api.SubmitRequirementResult]{
	Name: "submit_requirement", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_requirement",
	Doc:  "Submit a Requirement: its text becomes a Spec (titled with the first line) with three Ready Stage Tickets, grilling, then spec, then tickets, for a Factory to work; with map, a finished Map's Decisions replace the grilling Stage. Returns their acknowledgements, or the Spec and Tickets with full.",
}

// GetStageGuide is get_stage_guide: the Project's own guide, so its path
// takes the Project key. Its stages are the guide names, which
// domain.ModelKinds keeps in step with core's.
var GetStageGuide = &Definition[api.StageGuideInput, api.StageGuideView]{
	Name: "get_stage_guide", Audience: AgentAndHuman,
	HTTP:  "GET /api/p/{key}/stage-guides/{stage}",
	Doc:   "Get a Stage Guide again (claim returns it): what a Ticket of that Stage produces and how to finish it.",
	Enums: map[string][]string{"stage": domain.ModelKinds},
}

// SubmitMap is submit_map.
var SubmitMap = &Definition[api.SubmitMapInput, api.SubmitMapResult]{
	Name: "submit_map", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_map",
	Doc:  "Submit a Map: its text becomes a Spec labelled wayfinder:map (titled with the first line, with the Map's empty sections) and one Ready Charting Ticket for a Factory to work; returns their acknowledgements, or the Spec and Ticket with full.",
}

// SubmitPrototype is submit_prototype.
var SubmitPrototype = &Definition[api.SubmitPrototypeInput, api.Written[api.TicketView]]{
	Name: "submit_prototype", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_prototype",
	Doc:  "Submit a Prototype on any Spec: a Ready Ticket labelled prototype, titled \"Prototype: <first line>\" with the text as its description, for a Factory to build on a local branch that never lands; returns its acknowledgement, or the Ticket with full.",
}

// SubmitSmallChange is submit_small_change.
var SubmitSmallChange = &Definition[api.SubmitSmallChangeInput, api.Written[api.TicketView]]{
	Name: "submit_small_change", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_small_change",
	Doc:  "Submit a Small Change: a Ready implementation Ticket on the Project's Small Changes Spec, titled with the text's first line and carrying the text as its description, which a Factory builds and lands on its own; returns its acknowledgement, or the Ticket with full.",
}

// SubmitRetro is submit_retro.
var SubmitRetro = &Definition[api.SubmitRetroInput, api.Written[api.TicketView]]{
	Name: "submit_retro", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_retro",
	Doc:  "Start a Retro on finished work, a Done or Cancelled Ticket or a Finished Spec: a Ready Ticket labelled retro on the Project's Retros Spec, titled \"Retro: <target title>\", whose description lists every Claim on the work to read; returns its acknowledgement, or the Ticket with full.",
}

// OpenSpecMerge is the Factory's open_spec_merge (code-central-542): it
// asks Code Central to open the Ticket that merges the base into a
// conflicting Spec Branch.
var OpenSpecMerge = &Definition[api.OpenSpecMergeInput, api.OpenSpecMergeResult]{
	Name: "open_spec_merge", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/specs/{n}/open_spec_merge",
	Doc:  "Open the Ticket that merges the base into a conflicting Spec Branch.",
}

// NewTicket is the human's new_ticket (Spec code-central-1067).
var NewTicket = &Definition[api.NewTicketInput, api.Written[api.TicketView]]{
	Name: "new_ticket", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/new_ticket",
	Doc:  "Open a New Ticket on a Spec: a Ready implementation Ticket titled with the text's first line and carrying the text as its description; refused on a Map, a Long-lived Spec, a Requirement whose Stages aren't finished and in an Archived Project. Returns its acknowledgement, or the Ticket with full.",
}
