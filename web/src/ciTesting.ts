// Shared set-up for the tests of a CI failed item: Tickets pushed together
// as one commit whose CI failed, reported through the scenario builder, so
// each listed Ticket's own view and the item are the ones Core works out.
import type { FakeApi } from "./api/fake";
import type { Scenario } from "./api/scenario";
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
// (Accepted) pushed as abc1234567 and its run of ci.yml failed, and returns
// the one CI failed item listing them.
export async function failCI(s: Scenario, project: string, tickets: string[]): Promise<CIItem> {
  await s.trackCI(project);
  for (const id of tickets) await s.reportPushedCommit(project, id, "abc1234567", "main");
  await s.reportCIRuns(project, [failedRun(tickets[0])]);
  return (await s.api.getTicket(project, tickets[0])).ci_item!;
}

// failCIOnFake is failCI for the tests still on FakeApi, which also seeds
// Needs Me with the one CI failed item listing the Tickets.
export async function failCIOnFake(fake: FakeApi, project: string, tickets: string[]): Promise<CIItem> {
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
