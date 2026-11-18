package core

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/tomoram/code-central/domain"
)

// The Run summary (Spec code-central-1022): the part of a fix Ticket's
// description, and of the Send back to fix note, that Code Central writes
// from a pushed commit's CI. It names each failed run (each cancelled run,
// for CI cancelled) of the commit with its Workflow and failed jobs, the
// `gh run view` commands that read their logs, and the last passed commit
// in the Repository when there is one. No logs: the next agent reads them
// with gh.

// runSummary is the Run summary of it, a CI failed or CI cancelled item.
func (e *evaluation) runSummary(it ciItem) string {
	record := recordName(e.p.Key, it.repository)
	var runs []domain.CIRun
	for _, run := range e.ci.keptRuns() {
		if run.Ticket != 0 && run.Repository == record {
			runs = append(runs, run)
		}
	}
	cancelled := it.reason == ReasonCICancelled
	bad := func(c domain.CIConclusion) bool { return c.Failed() }
	if cancelled {
		bad = func(c domain.CIConclusion) bool { return c.Cancelled() }
	}
	verb := "failed"
	if cancelled {
		verb = "cancelled"
	}
	var badRuns []domain.CIRun
	for _, run := range countedRuns(it.commit, runs) {
		if run.Status == domain.CICompleted && bad(run.Conclusion) {
			badRuns = append(badRuns, run)
		}
	}
	slices.SortFunc(badRuns, func(a, b domain.CIRun) int {
		return cmp.Or(cmp.Compare(a.Workflow, b.Workflow), cmp.Compare(a.ID, b.ID))
	})
	var lines, logs []string
	for _, run := range badRuns {
		var jobs []string
		for _, j := range run.Jobs {
			if bad(j.Conclusion) {
				jobs = append(jobs, j.Name)
			}
		}
		slices.Sort(jobs)
		line := fmt.Sprintf("- [%s](%s) (%s)", cmp.Or(run.Name, run.Workflow), run.URL, run.Workflow)
		if len(jobs) > 0 {
			line += fmt.Sprintf(": %s jobs %s", verb, strings.Join(jobs, ", "))
		}
		lines = append(lines, line)
		logs = append(logs, fmt.Sprintf("`gh run view %d --log-failed`", run.ID))
	}
	parts := []string{fmt.Sprintf("CI %s at %s:", verb, it.label(e.p)), strings.Join(lines, "\n")}
	if len(logs) > 0 {
		parts = append(parts, fmt.Sprintf("Read the failed jobs' logs with %s.", strings.Join(logs, ", ")))
	}
	if last := e.lastPassedCommit(it.repository, runs); last != "" {
		parts = append(parts, fmt.Sprintf("The last commit that passed CI in this Repository is `%s`.", shortCommit(last)))
	}
	return strings.Join(parts, "\n\n")
}

// lastPassedCommit is the latest pushed commit in the Repository name,
// across the Project's Tickets, whose CI result is passed among runs, ""
// when there is none.
func (e *evaluation) lastPassedCommit(name string, runs []domain.CIRun) string {
	var last domain.PushedCommit
	for _, t := range e.evaluated().list {
		for _, pc := range t.PushedCommits {
			if recordRepository(e.p.Key, pc.Repository) != name || pc.Pushed.Before(last.Pushed) {
				continue
			}
			if ciResultOf(pc.Commit, runs) == CIPassed {
				last = pc
			}
		}
	}
	return last.Commit
}

// newFixNote is the note New fix Ticket starts from: the Run summary and
// the Project's Fix instructions.
func (e *evaluation) newFixNote(it ciItem) string {
	return e.runSummary(it) + "\n\n" + e.p.CI.AutoFix.FixInstructions()
}
