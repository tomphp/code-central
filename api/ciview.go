package api

import (
	"github.com/tomoram/code-central/domain"
)

// CIOverviewInput is the CI view's read: one Project, or every active
// Project when Project is "". Running keeps only the runs that count as
// running towards a Ticket's CI result, as the Activity Dashboard reads
// them (code-central-276).
type CIOverviewInput struct {
	Project string
	Running bool
}

// CIOverview is the CI view: the CI Watchers' statuses, then the runs,
// in-progress ones first, newest started first, then those finished in
// the last ciViewDays days, newest finished first. A run re-run lists its
// latest attempt only.
type CIOverview struct {
	Projects []CIProjectStatus `json:"projects"`
	Runs     []CIRunRow        `json:"runs"`
}

// CIProjectStatus is one Project's CI Tracking and its CI Watcher's
// status, as the CI view heads its runs with them.
type CIProjectStatus struct {
	Project    string            `json:"project"`
	Tracking   bool              `json:"tracking"`
	Provider   domain.CIProvider `json:"provider"`
	Repository string            `json:"repository"`
	ProjectURL string            `json:"project_url"`
	Watcher    WatcherView       `json:"watcher"`
}

// CIRunRow is one CI Run in the CI view, with its Project.
type CIRunRow struct {
	Project string `json:"project"`
	CIRunView
}
