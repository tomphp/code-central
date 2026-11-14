// Requirement tests: the real App on a WASM Core world. "New requirement",
// the Stage strip, the Requirement pane and the Requirements view.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario, WORK, type Scenario } from "../api/scenario";

const HOUR = 3_600_000;

function renderApp(w: World, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

// acme is a world with the Project acme, and the scenario builder on it.
async function acme(...more: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  for (const key of more) await s.project(key, key[0].toUpperCase() + key.slice(1));
  return { w, s };
}

// done takes a Stage Ticket to Done: finished with no branch, as a Stage
// is (a Spec Stage recommends implementing), and accepted.
async function done(s: Scenario, id: string) {
  const path = s.ticketPath("acme", id);
  const recommendation = (await s.api.getTicket("acme", id)).title.startsWith("Spec:") ? "implement" : undefined;
  await s.agent(`${path}/finish`, { claim_id: await s.claimFor("acme", id), summary: "Done the Stage", ...(recommendation && { recommendation }) });
  await s.human(`${path}/accept`, {});
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

describe("New requirement", () => {
  it("opens a modal with one text box, submits it and selects the new Requirement", async () => {
    const { w } = await acme();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Export a Spec as Markdown{Enter}One file.");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    expect(w.calls).toContain("submitRequirement");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(await within(detail()).findByText("Waiting for a Factory to claim Grilling. Nothing needs you yet.")).toBeInTheDocument();
    expect(within(detail()).getByRole("heading", { name: "Export a Spec as Markdown", level: 2 })).toBeInTheDocument();
    expect(params().get("requirement")).toBe("acme-1");
  });

  it("offers Straight to Spec, off by default, and submits it skipping Grilling", async () => {
    const { w } = await acme();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    const box = within(modal).getByRole("checkbox", { name: "Straight to Spec (no grilling)" });
    expect(box).not.toBeChecked();
    expect(within(modal).getByText(/Grilling, Spec and Tickets Stages/)).toBeInTheDocument();
    await userEvent.click(box);
    expect(within(modal).getByText("Creates a Spec and its Spec and Tickets Stages. A Factory picks up Spec.")).toBeInTheDocument();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Dark mode");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(await within(detail()).findByText(/Grilling/)).toBeInTheDocument();
    expect(w.calls).toContain("submitRequirement");
  });

  it("saves as Draft: Save as Draft writes every Stage Ticket as Draft", async () => {
    const { w } = await acme();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    expect(within(modal).getByRole("button", { name: "Save as Draft" })).toBeDisabled();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Export a Spec as Markdown");
    await userEvent.click(within(modal).getByRole("button", { name: "Save as Draft" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const tickets = (await w.api.findTickets("acme")).filter((t) => t.spec === "acme-1");
    expect(tickets.map((t) => t.state)).toEqual(["Draft", "Draft", "Draft"]);
  });

  it("closes on Cancel without submitting", async () => {
    const { w } = await acme();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).not.toContain("submitRequirement");
  });

  it("asks which Project when there are several and none is chosen, and goes to the chosen one", async () => {
    const { w } = await acme("beta");
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    await userEvent.selectOptions(within(modal).getByRole("combobox", { name: "Project" }), "beta");
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Beta work");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    await waitFor(() => expect(params().get("requirement")).toBe("beta-1"));
  });

  it("submits to the filtered Project without asking", async () => {
    const { w } = await acme("beta");
    renderApp(w, "/?view=inbox&project=beta");

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    expect(within(modal).queryByRole("combobox")).toBeNull();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "Beta work");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));
    await waitFor(() => expect(params().get("requirement")).toBe("beta-1"));
  });

  it("won't submit blank text", async () => {
    const { w } = await acme();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog");
    await userEvent.type(within(modal).getByRole("textbox", { name: "Requirement" }), "   ");
    expect(within(modal).getByRole("button", { name: "Submit requirement" })).toBeDisabled();
  });
});

// building is a Requirement in Build: every Stage Done, one implementation
// Ticket Done, one In Review (needing the human), one Ready, one Cancelled.
// It was submitted 2 hours ago.
async function building() {
  const { w, s } = await acme();
  const title = "Archive finished Projects";
  const r = await w.api.submitRequirement("acme", `${title}\nOnce everything is Done.`); // acme-1, stages acme-2..4
  for (const t of r.stages) await done(s, t.id);
  const spec = r.spec.id;
  await s.ticket("acme", { title: "store: archived flag", state: "Done", spec }); // acme-5
  const review = await s.ticket("acme", { title: "Archive in the Project menu", state: "In Review", spec }); // acme-6
  await s.ticket("acme", { title: "CLI: archive-project", state: "Ready", spec }); // acme-7
  await s.ticket("acme", { title: "Dropped idea", state: "Cancelled", spec }); // acme-8
  w.advance(2 * HOUR);
  return { w, s, review };
}

const pane = () => within(detail()).getByRole("region", { name: "Requirement" });
const segment = (name: RegExp) => within(within(pane()).getByRole("list", { name: "Stages" })).getByRole("button", { name });

describe("the Requirement pane", () => {
  it("shows Grilling skipped and Spec current for a Requirement submitted Straight to Spec", async () => {
    const { w } = await acme();
    await w.api.submitRequirement("acme", "Export a Spec as Markdown", undefined, true);
    renderApp(w, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    const strip = within(within(pane()).getByRole("list", { name: "Stages" }));
    expect(strip.getByText("Grilling").closest("li")).toHaveAttribute("data-progress", "skipped");
    expect(strip.queryByText("Map")).toBeNull();
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("data-progress", "queued");
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("aria-current", "step");
    expect(within(pane()).getByText("Waiting for a Factory to claim Spec. Nothing needs you yet.")).toBeInTheDocument();
  });

  it("shows the header and strip, a queued Stage dashed, while the Requirement waits for a Factory", async () => {
    const { w } = await acme();
    await w.api.submitRequirement("acme", "Export a Spec as Markdown");
    renderApp(w, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    expect(pane()).toHaveTextContent("acme-1");
    expect(pane()).toHaveTextContent("submitted just now");
    expect(segment(/^Grilling/).closest("li")).toHaveAttribute("data-progress", "queued");
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("data-progress", "waiting");
    expect(segment(/^Build/)).toBeDisabled();
    expect(within(pane()).getByText("Waiting for a Factory to claim Grilling. Nothing needs you yet.")).toBeInTheDocument();
    // With no Ticket needing the human, the Ticket pane opens on the current Stage.
    expect(await within(detail()).findByRole("heading", { name: "Grilling: Export a Spec as Markdown" })).toBeInTheDocument();
  });

  it("opens on the Ticket that needs the human, lists Build's Tickets with their States, and selects a Stage from the strip", async () => {
    const { w } = await building();
    renderApp(w, "/?view=inbox&requirement=acme-1");

    expect(await within(detail()).findByRole("heading", { name: "Archive in the Project menu" })).toBeInTheDocument();
    expect(pane()).toHaveTextContent("submitted 2h ago");
    expect(within(pane()).getByRole("heading", { name: "Archive finished Projects" })).toBeInTheDocument();
    expect(segment(/^Build/)).toHaveTextContent("Build 1/3");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("data-progress", "needs");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("aria-current", "step");
    // acme-7 can be claimed, but something needs the human.
    expect(within(pane()).getByText("Waiting for a Factory to claim a Build Ticket.")).toBeInTheDocument();

    const build = within(pane()).getByRole("list", { name: "Build" });
    const items = within(build).getAllByRole("listitem").map((li) => li.textContent);
    expect(items).toEqual([
      "acme-5 store: archived flag Done",
      "acme-6 Archive in the Project menu In Review",
      "acme-7 CLI: archive-project Ready",
      "acme-8 Dropped idea Cancelled",
    ]);

    await userEvent.click(segment(/^Spec/));
    expect(params().get("ticket")).toBe("acme-3");
    expect(params().get("requirement")).toBe("acme-1");
    expect(await within(detail()).findByRole("heading", { name: "Spec: Archive finished Projects" })).toBeInTheDocument();
    expect(segment(/^Spec/).closest("li")).toHaveAttribute("aria-current", "step");

    await userEvent.click(within(build).getByRole("button", { name: /acme-7 CLI/ }));
    expect(await within(detail()).findByRole("heading", { name: "CLI: archive-project" })).toBeInTheDocument();
  });

  it("shows an In Progress Build Ticket's latest Session Event faintly, and nothing on one with none", async () => {
    const { w, s } = await building();
    const spec = "acme-1";
    const busy = await s.ticket("acme", { title: "Archive in the API", state: "Ready", spec }); // acme-9
    const quiet = await s.ticket("acme", { title: "Archive in the store", state: "Ready", spec }); // acme-10
    await s.factorySession("acme", busy, { session: "s-busy", events: [{ time: new Date(w.now().getTime() - 5 * 60_000).toISOString(), kind: "tool_call", tool: "Edit", text: "Edit: core/archive.go" }] });
    await s.factorySession("acme", quiet, { session: "s-quiet" });
    renderApp(w, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    const build = within(pane()).getByRole("list", { name: "Build" });
    const items = within(build).getAllByRole("listitem");
    expect(within(items[4]).getByText("Edit: core/archive.go · 5m ago")).toHaveClass("latest-event");
    expect(items.map((li) => li.textContent)).toEqual([
      "acme-5 store: archived flag Done",
      "acme-6 Archive in the Project menu In Review",
      "acme-7 CLI: archive-project Ready",
      "acme-8 Dropped idea Cancelled",
      "acme-9 Archive in the API In ProgressEdit: core/archive.go · 5m ago",
      "acme-10 Archive in the store In Progress",
    ]);
  });

  it("updates an In Progress Build Ticket's latest Session Event on the live feed", async () => {
    const { w, s } = await building();
    const busy = await s.ticket("acme", { title: "Archive in the API", state: "Ready", spec: "acme-1" }); // acme-9
    await s.factorySession("acme", busy, { session: "s-busy" });
    renderApp(w, "/?view=inbox&requirement=acme-1");

    const build = await within(detail()).findByRole("list", { name: "Build" });
    await act(() => s.postSessionEvents("s-busy", [{ time: w.now().toISOString(), kind: "text", text: "Writing the test first." }]));
    expect(await within(build).findByText("Writing the test first. · just now")).toHaveClass("latest-event");
  });

  it("shows a Cancelled tickets Stage as skipped, makes Build current and opens on the implementation Ticket", async () => {
    const { w, s } = await acme();
    const r = await w.api.submitRequirement("acme", "Export a Spec as Markdown");
    await done(s, r.stages[0].id);
    await done(s, r.stages[1].id);
    await s.human(`${s.ticketPath("acme", r.stages[2].id)}/cancel`, {});
    await s.ticket("acme", { title: "Implement the export", state: "Ready", spec: r.spec.id });
    renderApp(w, "/?view=inbox&requirement=acme-1");

    await within(detail()).findByRole("region", { name: "Requirement" });
    const tickets = segment(/^Tickets/);
    expect(tickets.closest("li")).toHaveAttribute("data-progress", "skipped");
    expect(tickets).toHaveAttribute("title", "Tickets: skipped");
    expect(tickets).toBeEnabled();
    expect(within(pane()).getByText("Waiting for a Factory to claim a Build Ticket. Nothing needs you yet.")).toBeInTheDocument();
    expect(await within(detail()).findByRole("heading", { name: "Implement the export" })).toBeInTheDocument();

    await userEvent.click(tickets);
    expect(params().get("ticket")).toBe(r.stages[2].id);
  });

  it("says when Build waits for a Factory", async () => {
    const { w, s, review } = await building();
    await s.human(`${s.ticketPath("acme", review)}/accept`, {});
    await s.human(`${s.ticketPath("acme", review)}/land`, { commit: WORK.commit });
    renderApp(w, "/?view=inbox&requirement=acme-1");

    expect(await within(detail()).findByText("Waiting for a Factory to claim a Build Ticket. Nothing needs you yet.")).toBeInTheDocument();
    expect(segment(/^Build/)).toHaveTextContent("Build 2/3");
    expect(segment(/^Build/).closest("li")).toHaveAttribute("data-progress", "queued");
  });
});

describe("the Requirements view", () => {
  it("lists every Requirement with its strip, finished ones included, and opens one", async () => {
    const { w, s } = await building(); // acme-1
    const fresh = await w.api.submitRequirement("acme", "Export a Spec"); // acme-9
    const finished = await w.api.submitRequirement("acme", "Render Markdown"); // acme-13
    for (const t of finished.stages) await done(s, t.id);
    const notRequirement = await s.spec("acme", { title: "Not a Requirement" });
    await s.ticket("acme", { title: "Plain work", state: "Ready", spec: notRequirement });
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "Requirements 2" }));
    const list = screen.getByRole("main", { name: "Tickets" });
    const ids = () => [...list.querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
    await waitFor(() => expect(ids()).toEqual([fresh.spec.id, "acme-1", finished.spec.id]));

    const row = (id: string) => list.querySelector<HTMLElement>(`a[data-id="${id}"]`)!;
    expect(within(row("acme-1")).getByRole("list", { name: "Stages" })).toHaveTextContent("Build 1/3");
    expect(row("acme-1")).toHaveTextContent("Build: In Review");
    expect(within(row("acme-1")).getByLabelText("Needs you")).toBeInTheDocument();
    expect(row(fresh.spec.id)).toHaveTextContent("Grilling: waiting for a Factory");
    expect(row(finished.spec.id)).toHaveTextContent("Finished");
    expect(params().get("list")).toBe("requirements");

    await userEvent.click(row("acme-1"));
    expect(params().get("requirement")).toBe("acme-1");
    expect(await within(detail()).findByRole("heading", { name: "Archive in the Project menu" })).toBeInTheDocument();
    expect(row("acme-1")).toHaveAttribute("aria-current", "true");
  });
});
