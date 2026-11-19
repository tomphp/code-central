package core_test

import (
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"
)

// The Run summary (Spec code-central-1022): what the CI item's two
// prefilled notes start from, built in Go from the commit's CI.

// jobsRun reports, as the CI Watcher, attempt 1 of run id (below 10) of
// workflow on commit for ticket, completed with conclusion and the jobs
// given.
func (f fixture) jobsRun(t *testing.T, id int64, workflow, commit, ticket string, conclusion domain.CIConclusion, jobs ...api.CIJobView) {
	t.Helper()
	started := f.clock.Now()
	run := api.CIRunView{ID: id, Attempt: 1, Workflow: workflow, Name: strings.ToUpper(workflow[:2]), Branch: "main", Commit: commit,
		Status: domain.CICompleted, Conclusion: conclusion, Started: &started, Finished: &started,
		URL: "https://github.com/acme/web/actions/runs/" + string(rune('0'+id)), Ticket: ticket, Jobs: jobs}
	if _, err := f.core.ReportCIRuns(ctx, "acme", api.ReportCIRunsInput{Runs: []api.CIRunView{run}}); err != nil {
		t.Fatal(err)
	}
}

func TestTheRunSummaryNamesTheFailedRunsTheirFailedJobsAndHowToReadTheirLogs(t *testing.T) {
	f := itemWorld(t)
	f.moveTo(t, 8, domain.Done)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CIFailure,
		api.CIJobView{Name: "test", Conclusion: domain.CIFailure}, api.CIJobView{Name: "build", Conclusion: domain.CITimedOut}, api.CIJobView{Name: "lint", Conclusion: domain.CISuccess})
	f.jobsRun(t, 2, "e2e.yml", "abc123", "acme-5", domain.CISuccess)
	it := f.theItem(t, api.ReasonCIFailed).CIItem
	summary := "CI failed at abc123:\n\n- [CI](https://github.com/acme/web/actions/runs/1) (ci.yml): failed jobs build, test\n\n" +
		"Read the failed jobs' logs with `gh run view 1 --log-failed`."
	if it.SendBackNote != summary {
		t.Errorf("Send back to fix note %q, want the Run summary alone %q", it.SendBackNote, summary)
	}
	if want := summary + "\n\n" + domain.DefaultFixInstructions; it.NewFixNote != want {
		t.Errorf("New fix Ticket note %q, want the Run summary then the Fix instructions %q", it.NewFixNote, want)
	}
	if tk := f.getTicket(t, "acme-5"); tk.CIItem.NewFixNote != it.NewFixNote || tk.CIItem.SendBackNote != it.SendBackNote {
		t.Errorf("the Ticket's CI item carries notes %q and %q, want the Needs Me row's", tk.CIItem.NewFixNote, tk.CIItem.SendBackNote)
	}
}

func TestTheNewFixTicketNoteUsesTheProjectsFixInstructions(t *testing.T) {
	f := itemWorld(t)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CIFailure)
	if _, err := f.core.SetAutoFix(human, "acme", api.SetAutoFixInput{FixInstructions: "Fix it properly.", Limit: 3}); err != nil {
		t.Fatal(err)
	}
	it := f.theItem(t, api.ReasonCIFailed).CIItem
	if !strings.HasSuffix(it.NewFixNote, "\n\nFix it properly.") || strings.Contains(it.SendBackNote, "Fix it properly.") {
		t.Fatalf("notes %q and %q, want the Fix instructions on the New fix Ticket note only", it.NewFixNote, it.SendBackNote)
	}
}

func TestTheRunSummaryOfACancelledItemNamesItsCancelledRunsAndHasNoNewFixNote(t *testing.T) {
	f := itemWorld(t)
	f.moveTo(t, 8, domain.Done)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CICancelled, api.CIJobView{Name: "test", Conclusion: domain.CICancelled})
	it := f.theItem(t, api.ReasonCICancelled).CIItem
	want := "CI cancelled at abc123:\n\n- [CI](https://github.com/acme/web/actions/runs/1) (ci.yml): cancelled jobs test\n\n" +
		"Read the failed jobs' logs with `gh run view 1 --log-failed`."
	if it.SendBackNote != want || it.NewFixNote != "" {
		t.Fatalf("notes %q and %q, want the Run summary %q and no New fix Ticket note", it.SendBackNote, it.NewFixNote, want)
	}
}

func TestTheRunSummaryNamesTheLastPassedCommitWhenThereIsOne(t *testing.T) {
	f := itemWorld(t)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CISuccess)
	f.moveTo(t, 6, domain.Accepted)
	f.pushed(t, "acme-6", "def4567890")
	f.jobsRun(t, 2, "ci.yml", "def4567890", "acme-6", domain.CIFailure)
	it := f.theItem(t, api.ReasonCIFailed).CIItem
	if want := "CI failed at def4567:"; !strings.HasPrefix(it.SendBackNote, want) {
		t.Fatalf("note %q, want it to start %q", it.SendBackNote, want)
	}
	if want := "\n\nThe last commit that passed CI in this Repository is `abc123`."; !strings.HasSuffix(it.SendBackNote, want) {
		t.Fatalf("note %q, want it to end with the last passed commit", it.SendBackNote)
	}
}

func TestTheRunSummaryLeavesTheLastPassedCommitOutWhenNoneKnown(t *testing.T) {
	f := itemWorld(t)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CIFailure)
	if it := f.theItem(t, api.ReasonCIFailed).CIItem; strings.Contains(it.NewFixNote, "passed") {
		t.Fatalf("note %q, want no last passed commit line", it.NewFixNote)
	}
}

func TestAFixTicketsDescriptionIsTheHeaderThenTheNote(t *testing.T) {
	f := itemWorld(t)
	f.jobsRun(t, 1, "ci.yml", "abc123", "acme-5", domain.CIFailure)
	note := f.theItem(t, api.ReasonCIFailed).CIItem.NewFixNote + "\n\nAlso check the login test."
	if _, err := f.core.NewFixTicket(human, "acme", api.NewFixTicketInput{Ticket: "acme-8", Note: note}); err != nil {
		t.Fatal(err)
	}
	want := "acme-8's CI failed: fix the failure forward, on top of its work in the base. It stays Accepted, and lands when a later commit passes CI.\n\n" + note
	if got := f.getTicket(t, "acme-10").Description; got != want {
		t.Fatalf("description %q, want the header, then the edited note as it was given %q", got, want)
	}
}
