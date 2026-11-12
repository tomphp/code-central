// Land per Spec in the UI (code-central-381): the Waiting for the Spec badge,
// the Requirement's Land per Spec status and the per-Spec CI failed item,
// against FakeApi.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { App } from "./App";
import { FakeApi } from "./api/fake";

function renderAt(fake: FakeApi, url: string) {
  window.history.replaceState(null, "", url);
  render(<App api={fake} now={() => fake.now} />);
}

// world is a Requirement acme-1 in Build, under Merge with Land per Spec on:
// acme-5 and acme-6 are Accepted and waiting for the Spec, acme-7 is Ready.
async function world(ready = false) {
  const fake = new FakeApi();
  fake.seedOptions({ focus_clear: "all_clear" });
  fake.addProject("acme", "Acme");
  const r = await fake.submitRequirement("acme", "Export a Spec\nAs Markdown.");
  for (const t of r.stages) fake.updateTicket(t.id, { state: "Done" });
  const spec = { id: r.spec.id, title: "Export a Spec" };
  fake.addTicket("acme", { title: "Exporter", state: "Accepted", spec, waiting_for_spec: true }); // acme-5
  fake.addTicket("acme", { title: "Export button", state: "Accepted", spec, waiting_for_spec: true }); // acme-6
  fake.addTicket("acme", { title: "Export docs", state: ready ? "Accepted" : "Ready", spec, ...(ready && { waiting_for_spec: true }) }); // acme-7
  return fake;
}

describe("the Waiting for the Spec badge", () => {
  it("shows beside the State on the Ticket page", async () => {
    renderAt(await world(), "/?view=inbox&ticket=acme-5");
    const pane = await screen.findByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByText("Waiting for the Spec")).toBeInTheDocument();
  });

  it("shows in the Spec's Ticket list on the waiting Tickets only", async () => {
    renderAt(await world(), "/?requirement=acme-1");
    const list = await screen.findByRole("list", { name: "Build" });
    expect(within(list).getAllByText("Waiting for the Spec")).toHaveLength(2);
    const ready = within(list).getByText("Export docs").closest("li")!;
    expect(within(ready).queryByText("Waiting for the Spec")).toBeNull();
  });

  it("shows on the State list's rows", async () => {
    renderAt(await world(), "/?view=inbox&state=Accepted");
    expect((await screen.findAllByText("Waiting for the Spec")).length).toBeGreaterThanOrEqual(2);
  });
});

describe("the Requirement's Land per Spec status", () => {
  it("counts the Tickets waiting", async () => {
    renderAt(await world(), "/?requirement=acme-1");
    expect(await screen.findByRole("status", { name: "Land per Spec" })).toHaveTextContent("2 Tickets waiting for the Spec");
  });

  it("says merging once the Spec is ready", async () => {
    renderAt(await world(true), "/?requirement=acme-1");
    expect(await screen.findByRole("status", { name: "Land per Spec" })).toHaveTextContent("Merging into the base");
  });
});

describe("the CI item of a Spec-landed Ticket", () => {
  it("lists the other Tickets waiting on the Spec's pushed commit, offering no Send back to fix", async () => {
    const fake = await world(true);
    await fake.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    await fake.reportPushedCommit("acme", "acme-5", "abc1234567");
    await fake.reportCIRuns("acme", [
      { id: 1, attempt: 1, workflow: "ci.yml", name: "CI", branch: "main", commit: "abc1234567", status: "completed", conclusion: "failure", url: "https://example.test/1", jobs: [], ticket: "acme-5" },
    ]);
    const tickets = ["acme-5", "acme-6", "acme-7"].map((id) => ({ id, title: id, spec: "acme-1" }));
    const options = ["rerun_ci", "land_anyway", "new_fix_ticket"] as const;
    fake.updateTicket("acme-5", { ci_item: { reason: "ci_failed", commit: "abc1234567", tickets, options: [...options] }, ci_failure_options: [...options] });
    fake.setNeedsMe([{ reason: "ci_failed", tickets: [{ id: "", project: "acme", title: "CI failed at abc1234", ci_item: { reason: "ci_failed", commit: "abc1234567", tickets, options: [...options] } }] }]);
    renderAt(fake, "/?view=inbox&ticket=acme-5");
    const pane = await screen.findByRole("complementary", { name: "Ticket" });
    const panel = await within(pane).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/is one Needs Me item, listing 3 Tickets/)).toHaveTextContent("Also waiting: acme-6, acme-7");
    const group = within(panel).getByRole("group", { name: "Failure options" });
    expect(within(group).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: "Land anyway" })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /New fix Ticket/ })).toBeInTheDocument();
    expect(within(group).queryByRole("button", { name: /Send back to fix/ })).toBeNull();
  });
});
