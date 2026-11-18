package ops

import "github.com/tomoram/code-central/core"

// Submitting work and the Stage Guides. The submits write as human
// whatever Actor the request names; core sees to that.

// SubmitRequirement is submit_requirement.
var SubmitRequirement = &Definition[core.SubmitRequirementInput, core.SubmitRequirementResult]{
	Name: "submit_requirement", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_requirement",
	Doc:  "Submit a Requirement: its text becomes a Spec (titled with the first line) with three Ready Stage Tickets, grilling, then spec, then tickets, for a Factory to work; with map, a finished Map's Decisions replace the grilling Stage. Returns their acknowledgements, or the Spec and Tickets with full.",
	Call: (*core.Core).SubmitRequirement,
}

// GetStageGuide is get_stage_guide: the Project's own guide, so its path
// takes the Project key.
var GetStageGuide = &Definition[core.StageGuideInput, core.StageGuideView]{
	Name: "get_stage_guide", Audience: AgentAndHuman,
	HTTP:  "GET /api/p/{key}/stage-guides/{stage}",
	Doc:   "Get a Stage Guide again (claim returns it): what a Ticket of that Stage produces and how to finish it.",
	Enums: map[string][]string{"stage": core.GuideNames},
	Call:  (*core.Core).GetStageGuide,
}

// SubmitMap is submit_map.
var SubmitMap = &Definition[core.SubmitMapInput, core.SubmitMapResult]{
	Name: "submit_map", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_map",
	Doc:  "Submit a Map: its text becomes a Spec labelled wayfinder:map (titled with the first line, with the Map's empty sections) and one Ready Charting Ticket for a Factory to work; returns their acknowledgements, or the Spec and Ticket with full.",
	Call: (*core.Core).SubmitMap,
}

// SubmitPrototype is submit_prototype.
var SubmitPrototype = &Definition[core.SubmitPrototypeInput, core.Written[core.TicketView]]{
	Name: "submit_prototype", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_prototype",
	Doc:  "Submit a Prototype on any Spec: a Ready Ticket labelled prototype, titled \"Prototype: <first line>\" with the text as its description, for a Factory to build on a local branch that never lands; returns its acknowledgement, or the Ticket with full.",
	Call: (*core.Core).SubmitPrototype,
}

// SubmitSmallChange is submit_small_change.
var SubmitSmallChange = &Definition[core.SubmitSmallChangeInput, core.Written[core.TicketView]]{
	Name: "submit_small_change", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_small_change",
	Doc:  "Submit a Small Change: a Ready implementation Ticket on the Project's Small Changes Spec, titled with the text's first line and carrying the text as its description, which a Factory builds and lands on its own; returns its acknowledgement, or the Ticket with full.",
	Call: (*core.Core).SubmitSmallChange,
}

// SubmitRetro is submit_retro.
var SubmitRetro = &Definition[core.SubmitRetroInput, core.Written[core.TicketView]]{
	Name: "submit_retro", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/submit_retro",
	Doc:  "Start a Retro on finished work, a Done or Cancelled Ticket or a Finished Spec: a Ready Ticket labelled retro on the Project's Retros Spec, titled \"Retro: <target title>\", whose description lists every Claim on the work to read; returns its acknowledgement, or the Ticket with full.",
	Call: (*core.Core).SubmitRetro,
}

// OpenSpecMerge is the Factory's open_spec_merge (code-central-542): it
// asks Code Central to open the Ticket that merges the base into a
// conflicting Spec Branch.
var OpenSpecMerge = &Definition[core.OpenSpecMergeInput, core.OpenSpecMergeResult]{
	Name: "open_spec_merge", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/specs/{n}/open_spec_merge",
	Doc:  "Open the Ticket that merges the base into a conflicting Spec Branch.",
	Call: (*core.Core).OpenSpecMerge,
}
