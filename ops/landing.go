package ops

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// Landing and Repositories.

// Land is land.
var Land = &Definition[api.LandInput, api.Written[api.TicketView]]{
	Name: "land", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/land",
	Doc:  "Land a Ticket: once its work is merged into the base of one of its Changed Repositories, record the commit that put it there; once it has landed in every one, it moves from Accepted (or In Review) to Done. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).Land,
}

// SetLandingMethod is set_landing_method.
var SetLandingMethod = &Definition[api.SetLandingMethodInput, api.ProjectView]{
	Name: "set_landing_method", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_landing_method",
	Doc:  "Set how the Project's accepted work lands: the Landing Method, Push, remote, Land per Spec, Rebase, the PAT variable and the base.",
	Call: (*core.Core).SetLandingMethod,
}

// AddRepository is add_repository.
var AddRepository = &Definition[api.AddRepositoryInput, api.Written[api.ProjectView]]{
	Name: "add_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/add_repository",
	Doc:  "Add a Repository to the Project, at the end of its landing order.",
	Call: (*core.Core).AddRepository,
}

// EditRepository is edit_repository.
var EditRepository = &Definition[api.EditRepositoryInput, api.Written[api.ProjectView]]{
	Name: "edit_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/edit_repository",
	Doc:  "Edit a Repository's Clone URL, base, local path, landing or push remote.",
	Call: (*core.Core).EditRepository,
}

// RemoveRepository is remove_repository.
var RemoveRepository = &Definition[api.RemoveRepositoryInput, api.Written[api.ProjectView]]{
	Name: "remove_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/remove_repository",
	Doc:  "Remove a Repository from the Project.",
	Call: (*core.Core).RemoveRepository,
}

// SetLandingOrder is set_landing_order.
var SetLandingOrder = &Definition[api.SetLandingOrderInput, api.Written[api.ProjectView]]{
	Name: "set_landing_order", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_landing_order",
	Doc:  "Set the Repositories' landing order: every Repository Name once.",
	Call: (*core.Core).SetLandingOrder,
}

// ReportRepositories is report_repositories.
var ReportRepositories = &Definition[api.ReportRepositoriesInput, api.Written[api.ProjectView]]{
	Name: "report_repositories", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_repositories",
	Doc:  "The Factory's report of each Repository's clone, ready or missing.",
	Call: (*core.Core).ReportRepositories,
}

// ReportChangedRepositories is report_changed_repositories.
var ReportChangedRepositories = &Definition[api.ReportChangedRepositoriesInput, api.Written[api.TicketView]]{
	Name: "report_changed_repositories", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_changed_repositories",
	Doc:  "The Factory's live report of the Repositories whose branch holds a Ticket's work, each with its branch and head commit.",
	Call: (*core.Core).ReportChangedRepositories,
}

// FailFinish is fail_finish.
var FailFinish = &Definition[api.FailFinishInput, api.Written[api.TicketView]]{
	Name: "fail_finish", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/fail_finish",
	Doc:  "The Factory's Failure Report on a finish its finish check found wrong.",
	Call: (*core.Core).FailFinish,
}
