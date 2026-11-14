// CI items (code-central-432): CI failed, CI cancelled and CI unavailable
// are one Needs Me item per pushed commit, with no Ticket of its own,
// listing the Tickets waiting on it across Specs. Against FakeApi: the
// Inbox row, its list and badge; the Focus Item with the list and the
// failure options, which act on every listed Ticket; and a listed
// Ticket's own view, which links the item.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "./api/fake";
import type { CIFailureOption, CIItem } from "./api/types";
import { failCI, failedRun } from "./ciTesting";
import { allClearOn, focus, renderApp } from "./focus/focusTesting";
import { rowDetail } from "./format";

// world has acme-1 (on acme's first Spec) and acme-3 (on another) Accepted
// under Merge + Push with CI Tracking, both pushed as abc1234567, whose
// one run of ci.yml failed: one CI failed item listing both.
async function world() {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Login page", state: "Accepted", branch: "acme-1", commit: "abc" }); // acme-1
  fake.addSpec("acme", { title: "Shop" }); // acme-2
  fake.addTicket("acme", { title: "Basket", state: "Accepted", branch: "acme-3", commit: "def", spec: { id: "acme-2", title: "Shop" } }); // acme-3
  const item = await failCI(fake, "acme", ["acme-1", "acme-3"]);
  return { fake, item };
}

describe("a CI item in Needs Me", () => {
  it("is one row per pushed commit with no Ticket, listing the Tickets waiting on it across Specs", async () => {
    const { fake, item } = await world();
    expect(item).toMatchObject({ reason: "ci_failed", commit: "abc1234567", options: ["rerun_ci", "land_anyway", "new_fix_ticket"] });
    expect(item.tickets).toEqual([
      { id: "acme-1", title: "Login page", spec: expect.any(String) },
      { id: "acme-3", title: "Basket", spec: "acme-2" },
    ]);
    const row = (await fake.needsMe()).groups[0].tickets[0];
    expect(rowDetail(fake.now, row)).toBe("acme-1, acme-3 · ci.yml failed");

    renderApp(fake, "/?view=inbox");
    const group = await within(screen.getByRole("main", { name: "Tickets" })).findByRole("region", { name: "CI failed" });
    expect(within(group).getByRole("heading", { level: 3 })).toHaveTextContent("CI failed 1");
    // The row opens on the first Ticket it lists, whose panel shows the item.
    const link = within(group).getByRole("link", { name: /CI failed at abc1234/ });
    expect(link).toHaveAttribute("href", expect.stringContaining("ticket=acme-1"));
    expect(within(link).getByText("acme")).toHaveClass("tid");
    expect(within(link).getByText("acme-1, acme-3 · ci.yml failed")).toBeInTheDocument();
    expect(within(link).getByText("CI failed", { selector: ".ci-badge" })).toBeInTheDocument();
  });

  it("is one Focus Item whose panel lists the Tickets and whose options act on all of them", async () => {
    const { fake } = await world();
    renderApp(fake);
    const focusItem = await within(focus()).findByRole("article", { name: "Focus Item" });
    expect(within(focusItem).getByRole("heading", { level: 2 })).toHaveTextContent("CI failed at abc1234");
    expect(within(focusItem).getByText("Focus: CI failed")).toBeInTheDocument();
    const panel = await within(focusItem).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/is one Needs Me item, listing 2 Tickets/)).toHaveTextContent("Also waiting: acme-3");
    expect(within(panel).getByRole("link", { name: "acme-3" })).toHaveAttribute("href", expect.stringContaining("ticket=acme-3"));
    const options = within(panel).getByRole("group", { name: "Failure options" });
    expect(within(options).queryByRole("button", { name: /Send back to fix/ })).toBeNull();
    await userEvent.click(within(options).getByRole("button", { name: "Land anyway" }));
    for (const id of ["acme-1", "acme-3"]) {
      expect((await fake.getTicket("acme", id)).land_anyway).toBe(true);
    }
    expect((await fake.needsMe()).count).toBe(0);
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
  });

  it("is linked from each listed Ticket's own view, which shows its CI result", async () => {
    const { fake } = await world();
    renderApp(fake, "/?view=inbox&ticket=acme-3");
    const pane = await screen.findByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByText("CI failed", { selector: ".reason" })).toBeInTheDocument();
    const panel = await within(pane).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/is one Needs Me item/, { selector: ".ci-item" })).toHaveTextContent(
      "CI failed at abc1234 is one Needs Me item, listing 2 Tickets waiting on the commit. Your choice acts on all of them. Also waiting: acme-1.",
    );
    expect(within(panel).getByRole("link", { name: "acme-1" })).toHaveAttribute("href", expect.stringContaining("ticket=acme-1"));
    expect(within(within(panel).getByRole("region", { name: "CI" })).getByRole("heading", { level: 3 })).toHaveTextContent("CI failed abc1234");
  });

  it("offers Send back to fix only when it lists one Ticket", async () => {
    const { fake } = await world();
    fake.updateTicket("acme-3", { state: "Done" });
    await fake.reportCIRuns("acme", [failedRun("acme-1")]);
    const item = (await fake.getTicket("acme", "acme-1")).ci_item!;
    expect(item.tickets.map((t) => t.id)).toEqual(["acme-1"]);
    expect(item.options).toEqual(["rerun_ci", "land_anyway", "send_back", "new_fix_ticket"]);
  });

  it("shows a CI unavailable item's cause and its Tickets, with no options", async () => {
    const fake = new FakeApi();
    allClearOn(fake);
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Login page", state: "Accepted", ci_result: "running" }); // acme-1
    fake.addTicket("acme", { title: "Basket", state: "Accepted", ci_result: "running" }); // acme-2
    const options: CIFailureOption[] = [];
    const ci_item: CIItem = { reason: "ci_unavailable", commit: "abc1234567", tickets: [{ id: "acme-1", title: "Login page", spec: "acme-1" }, { id: "acme-2", title: "Basket", spec: "acme-1" }], options };
    fake.updateTicket("acme-1", { ci_item, needs_me_reason: "ci_unavailable" });
    const pushed = new Date(fake.now.getTime() - 12 * 60_000).toISOString();
    fake.setNeedsMe([
      { reason: "ci_unavailable", tickets: [{ id: "", project: "acme", title: "CI unavailable at abc1234", since: pushed, ci_unavailable: { cause: "no_ci_runs", commit: "abc1234567", pushed }, ci_item }] },
    ]);
    const row = (await fake.needsMe()).groups[0].tickets[0];
    expect(rowDetail(fake.now, row)).toBe("acme-1, acme-2 · No CI Runs reported for abc1234 pushed 12m ago");

    renderApp(fake);
    const focusItem = await within(focus()).findByRole("article", { name: "Focus Item" });
    const panel = await within(focusItem).findByRole("region", { name: "CI unavailable" });
    expect(within(panel).getByText(/is one Needs Me item, listing 2 Tickets/)).toHaveTextContent("Also waiting: acme-2");
    expect(within(panel).queryByRole("group", { name: "Failure options" })).toBeNull();
  });
});
