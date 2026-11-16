// Land per Spec in the UI (code-central-381): the Waiting for the Spec badge,
// the Requirement's Land per Spec status and the per-Spec CI failed item,
// on the WASM Core.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { allClear, renderApp, scenario } from "./worldTesting";

// world is a Requirement in Build, under Merge with Land per Spec on: two
// Tickets are Accepted and waiting for the Spec, and a third is Ready, or
// Accepted too when ready. The IDs are the ones the writes returned.
async function world(ready = false) {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.landOn("acme", "merge", false);
  const spec = await s.requirement("acme", "Export a Spec\nAs Markdown.");
  const exporter = await s.ticket("acme", { title: "Exporter", state: "Accepted", spec });
  const button = await s.ticket("acme", { title: "Export button", state: "Accepted", spec });
  const docs = await s.ticket("acme", { title: "Export docs", state: ready ? "Accepted" : "Ready", spec });
  if (ready) for (const id of [exporter, button, docs]) await s.specMerged("acme", id);
  return { w, s, spec, exporter, button, docs };
}

describe("the Waiting for the Spec badge", () => {
  it("shows beside the State on the Ticket page", async () => {
    const { w, exporter } = await world();
    await renderApp(w, `/?view=inbox&ticket=${exporter}`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(pane).getByText("Waiting for the Spec")).toBeInTheDocument();
  });

  it("shows in the Spec's Ticket list on the waiting Tickets only", async () => {
    const { w, spec } = await world();
    await renderApp(w, `/?requirement=${spec}`);
    const list = screen.getByRole("list", { name: "Build" });
    expect(within(list).getAllByText("Waiting for the Spec")).toHaveLength(2);
    const ready = within(list).getByText("Export docs").closest("li")!;
    expect(within(ready).queryByText("Waiting for the Spec")).toBeNull();
  });

  it("shows on the State list's rows", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=inbox&state=Accepted");
    expect((screen.getAllByText("Waiting for the Spec")).length).toBeGreaterThanOrEqual(2);
  });
});

describe("the Requirement's Land per Spec status", () => {
  it("counts the Tickets waiting", async () => {
    const { w, spec } = await world();
    await renderApp(w, `/?requirement=${spec}`);
    expect(screen.getByRole("status", { name: "Land per Spec" })).toHaveTextContent("2 Tickets waiting for the Spec");
  });

  it("says merging once the Spec is ready", async () => {
    const { w, spec } = await world(true);
    await renderApp(w, `/?requirement=${spec}`);
    expect(screen.getByRole("status", { name: "Land per Spec" })).toHaveTextContent("Merging into the base");
  });
});

describe("the CI item of a Spec-landed Ticket", () => {
  it("lists the other Tickets waiting on the Spec's pushed commit, offering no Send back to fix", async () => {
    const { w, s, exporter, button, docs } = await world(true);
    // The Spec's merge is pushed once, as the commit of each of its Tickets.
    for (const id of [exporter, button, docs]) await s.pushed("acme", id, "abc1234567");
    await s.ci("acme", exporter, "failed", "abc1234567");
    await renderApp(w, `/?view=inbox&ticket=${exporter}`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    const panel = within(pane).getByRole("region", { name: "CI failed" });
    expect(within(panel).getByText(/is one Needs Me item, listing 3 Tickets/)).toHaveTextContent(`Also waiting: ${button}, ${docs}`);
    const group = within(panel).getByRole("group", { name: "Failure options" });
    expect(within(group).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: "Land anyway" })).toBeInTheDocument();
    expect(within(group).getByRole("button", { name: /New fix Ticket/ })).toBeInTheDocument();
    expect(within(group).queryByRole("button", { name: /Send back to fix/ })).toBeNull();
  });
});
