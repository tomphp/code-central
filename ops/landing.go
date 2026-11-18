package ops

import "github.com/tomoram/code-central/core"

// Landing and Repositories.

// Land is land.
var Land = &Definition[core.LandInput, core.Written[core.TicketView]]{
	Name: "land", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/land",
	Doc:  "Land a Ticket: once its work is merged into the base of one of its Changed Repositories, record the commit that put it there; once it has landed in every one, it moves from Accepted (or In Review) to Done. Returns an acknowledgement with the Ticket's State, or the Ticket with full.",
	Call: (*core.Core).Land,
}

// SetLandingMethod is set_landing_method.
var SetLandingMethod = &Definition[core.SetLandingMethodInput, core.ProjectView]{
	Name: "set_landing_method", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_landing_method",
	Doc:  "Set how the Project's accepted work lands: the Landing Method, Push, remote, Land per Spec, the PAT variable and the base.",
	Call: (*core.Core).SetLandingMethod,
}

// AddRepository is add_repository.
var AddRepository = &Definition[core.AddRepositoryInput, core.Written[core.ProjectView]]{
	Name: "add_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/add_repository",
	Doc:  "Add a Repository to the Project, at the end of its landing order.",
	Call: (*core.Core).AddRepository,
}

// EditRepository is edit_repository.
var EditRepository = &Definition[core.EditRepositoryInput, core.Written[core.ProjectView]]{
	Name: "edit_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/edit_repository",
	Doc:  "Edit a Repository's Clone URL, base, local path, landing or push remote.",
	Call: (*core.Core).EditRepository,
}

// RemoveRepository is remove_repository.
var RemoveRepository = &Definition[core.RemoveRepositoryInput, core.Written[core.ProjectView]]{
	Name: "remove_repository", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/remove_repository",
	Doc:  "Remove a Repository from the Project.",
	Call: (*core.Core).RemoveRepository,
}

// SetLandingOrder is set_landing_order.
var SetLandingOrder = &Definition[core.SetLandingOrderInput, core.Written[core.ProjectView]]{
	Name: "set_landing_order", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/set_landing_order",
	Doc:  "Set the Repositories' landing order: every Repository Name once.",
	Call: (*core.Core).SetLandingOrder,
}

// ReportRepositories is report_repositories.
var ReportRepositories = &Definition[core.ReportRepositoriesInput, core.Written[core.ProjectView]]{
	Name: "report_repositories", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_repositories",
	Doc:  "The Factory's report of each Repository's clone, ready or missing.",
	Call: (*core.Core).ReportRepositories,
}

// ReportChangedRepositories is report_changed_repositories.
var ReportChangedRepositories = &Definition[core.ReportChangedRepositoriesInput, core.Written[core.TicketView]]{
	Name: "report_changed_repositories", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_changed_repositories",
	Doc:  "The Factory's live report of the Repositories whose branch holds a Ticket's work, each with its branch and head commit.",
	Call: (*core.Core).ReportChangedRepositories,
}

// FailFinish is fail_finish.
var FailFinish = &Definition[core.FailFinishInput, core.Written[core.TicketView]]{
	Name: "fail_finish", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/fail_finish",
	Doc:  "The Factory's Failure Report on a finish its finish check found wrong.",
	Call: (*core.Core).FailFinish,
}
