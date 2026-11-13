// Shared set-up for the tests of a CI failed item: Tickets pushed together
// as one commit whose CI failed, through FakeApi's CI path, so each listed
// Ticket's own view and the item agree as they would from the service.
import type { FakeApi } from "./api/fake";
import type { CIItem } from "./api/types";

// failedRun is ci.yml's one run of abc1234567 on main, failed, reported for
// ticket.
export const failedRun = (ticket: string) => ({
  id: 1,
  attempt: 1,
  workflow: "ci.yml",
  name: "CI",
  branch: "main",
  commit: "abc1234567",
  status: "completed" as const,
  conclusion: "failure" as const,
  url: "https://github.com/acme/web/actions/runs/1",
  jobs: [{ name: "test", conclusion: "failure" as const, url: "https://github.com/acme/web/actions/runs/1/job/1" }],
  ticket,
});

// failCI puts project under Merge + Push with CI Tracking, reports tickets
// (Accepted, each with a branch) pushed as abc1234567 and its run of
// ci.yml failed, and seeds Needs Me with the one CI failed item listing
// them, which it returns.
export async function failCI(fake: FakeApi, project: string, tickets: string[]): Promise<CIItem> {
  await fake.setLandingMethod(project, { method: "merge", push: true, remote: "origin" });
  await fake.setCITracking(project, { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
  for (const id of tickets) await fake.reportPushedCommit(project, id, "abc1234567", "main");
  await fake.reportCIRuns(project, [failedRun(tickets[0])]);
  const item = (await fake.getTicket(project, tickets[0])).ci_item!;
  fake.setNeedsMe([
    { reason: "ci_failed", tickets: [{ id: "", project, title: "CI failed at abc1234", ci_result: "failed", ci_failure: { commit: "abc1234567", workflows: ["ci.yml"] }, ci_item: item }] },
  ]);
  return item;
}
