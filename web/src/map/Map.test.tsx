// Map tests: the real App against FakeApi. "New Map", the Map view and
// the Map's row in the Requirements view.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { MAP_LABEL, type State } from "../api/types";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const list = () => screen.getByRole("main", { name: "Tickets" });
const params = () => new URLSearchParams(window.location.search);
const row = (id: string) => list().querySelector<HTMLElement>(`a[data-id="${id}"]`)!;

describe("New Map", () => {
  it("opens from the + New menu, submits the same free-text dialog and selects the new Map", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "More to create" }));
    await userEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Map" }));
    const modal = screen.getByRole("dialog", { name: "New Map" });
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    expect(within(modal).getByRole("button", { name: "Submit map" })).toBeDisabled();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Map" }), "Offline mode{Enter}Works on a plane.");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit map" }));

    expect(fake.calls).toContain("submitMap");
    expect(fake.calls).not.toContain("submitRequirement");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(params().get("requirement")).toBe("acme-1");
    const pane = await within(detail()).findByRole("region", { name: "Map" });
    expect(within(pane).getByRole("heading", { name: "Offline mode", level: 2 })).toBeInTheDocument();
    expect(pane).toHaveTextContent("0/1 decided");
    const charting = within(within(pane).getByRole("list", { name: "Map Tickets" })).getByRole("listitem");
    expect(charting).toHaveTextContent("chart");
    expect(charting).toHaveTextContent("Ready");
    expect(charting).toHaveTextContent("acme-2 Charting: Offline mode");
    expect(row("acme-1")).toHaveTextContent("0/1 decided");
  });
});

describe("a Map's row in the Requirements view", () => {
  it("shows the decided count in place of the strip, and what needs you only when something does", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const quiet = await fake.submitMap("acme", "Offline mode"); // acme-1, Charting acme-2
    const busy = await fake.submitMap("acme", "Sync"); // acme-3, Charting acme-4
    const spec = { id: busy.spec.id, title: "Sync" };
    fake.updateTicket(busy.charting.id, { state: "Done" });
    fake.addTicket("acme", { title: "Which store", state: "In Review", spec, labels: ["wayfinder:grilling"] }); // acme-5
    fake.addTicket("acme", { title: "Try a cache", state: "In Review", spec, labels: ["prototype"] }); // acme-6
    fake.addTicket("acme", { title: "Dropped", state: "Cancelled", spec, labels: ["wayfinder:task"] }); // acme-7
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-5" }, { id: "acme-6" }] }]);
    const req = await fake.submitRequirement("acme", "Export a Spec"); // acme-8
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "Requirements 3" }));
    const ids = () => [...list().querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
    await waitFor(() => expect(ids()).toEqual([req.spec.id, busy.spec.id, quiet.spec.id]));

    expect(within(row(busy.spec.id)).queryByRole("list", { name: "Stages" })).toBeNull();
    expect(row(busy.spec.id)).toHaveTextContent("1/2 decided · 2 needs you");
    expect(within(row(busy.spec.id)).getByLabelText("Needs you")).toBeInTheDocument();
    expect(row(quiet.spec.id)).toHaveTextContent("0/1 decided");
    expect(row(quiet.spec.id)).not.toHaveTextContent("needs you");
    expect(within(row(quiet.spec.id)).queryByLabelText("Needs you")).toBeNull();
    expect(within(row(req.spec.id)).getByRole("list", { name: "Stages" })).toBeInTheDocument();
  });
});

// aMap seeds Map acme-1 with a body under the fixed headings and these
// Tickets, acme-2 on, each with its Label, State and Blockers.
function aMap(fake: FakeApi, tickets: { title: string; label: string; state: State; blockedBy?: string[] }[]) {
  fake.addProject("acme", "Acme");
  const body = [
    "Offline mode",
    "## Destination",
    "The app works on a plane.",
    "## Notes",
    "## Decisions so far",
    "- Store edits locally, from [acme-2](acme-2)",
    "## Not yet specified",
    "How conflicts merge.",
    "## Out of scope",
    "Offline sign-up.",
  ].join("\n\n");
  const spec = fake.addSpec("acme", { title: "Offline mode", labels: [MAP_LABEL], body });
  for (const t of tickets) {
    const blockedBy = t.blockedBy ?? [];
    const blockers = blockedBy.map((id) => fake.ticketsIn("acme").find((b) => b.id === id)!);
    fake.addTicket("acme", {
      title: t.title,
      state: t.state,
      spec: { id: spec.id, title: spec.title },
      labels: [t.label],
      blocked_by: blockedBy,
      blockers: blockers.map((b) => ({ id: b.id, title: b.title, state: b.state })),
      blocked: blockers.some((b) => b.state !== "Done"),
    });
  }
}

// The Map below: Charting is done; Which store is the frontier and
// blocks Sync libraries; Ask legal runs; Try a cache needs the human;
// Pick a format is queued for a Factory.
const offlineMode = (fake: FakeApi) =>
  aMap(fake, [
    { title: "Charting: Offline mode", label: "wayfinder:chart", state: "Done" }, // acme-2
    { title: "Which store", label: "wayfinder:grilling", state: "Draft", blockedBy: ["acme-2"] }, // acme-3
    { title: "Sync libraries", label: "wayfinder:research", state: "Draft", blockedBy: ["acme-3"] }, // acme-4
    { title: "Ask legal", label: "wayfinder:task", state: "In Progress", blockedBy: ["acme-2"] }, // acme-5
    { title: "Try a cache", label: "prototype", state: "In Review" }, // acme-6
    { title: "Pick a format", label: "wayfinder:grilling", state: "Ready", blockedBy: ["acme-2"] }, // acme-7
  ]);

describe("the Map view", () => {
  const open = async (fake: FakeApi) => {
    renderApp(fake, "/?view=inbox&list=requirements&requirement=acme-1");
    return within(detail()).findByRole("region", { name: "Map" });
  };
  const graph = (pane: HTMLElement) => within(pane).getByRole("list", { name: "Map Tickets" });
  const node = (pane: HTMLElement, id: string) => graph(pane).querySelector<HTMLElement>(`[data-id="${id}"]`)!;

  it("replaces the Requirement view with a left-to-right graph of the Map's Tickets, coloured by progress, with Blockers as edges", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-6" }] }]);
    const pane = await open(fake);

    expect(within(detail()).queryByRole("region", { name: "Requirement" })).toBeNull();
    expect(within(detail()).queryByRole("list", { name: "Stages" })).toBeNull();
    expect(node(pane, "acme-3")).toHaveTextContent("grilling");
    expect(node(pane, "acme-3")).toHaveTextContent("Which store");
    expect(node(pane, "acme-4")).toHaveTextContent("research");
    expect(node(pane, "acme-6")).toHaveTextContent("prototype");
    const progress = Object.fromEntries(["acme-2", "acme-3", "acme-4", "acme-5", "acme-6", "acme-7"].map((id) => [id, node(pane, id).dataset.progress]));
    expect(progress).toEqual({
      "acme-2": "done",
      "acme-3": "waiting",
      "acme-4": "waiting",
      "acme-5": "running",
      "acme-6": "needs",
      "acme-7": "queued",
    });

    const edges = [...pane.querySelectorAll<SVGElement>("[data-edge]")].map((e) => e.dataset.edge).sort();
    expect(edges).toEqual(["acme-2>acme-3", "acme-2>acme-5", "acme-2>acme-7", "acme-3>acme-4"]);
    // Left to right: a Blocker sits left of what it blocks, and the fog node right of all.
    const x = (id: string) => Number(node(pane, id).getAttribute("x"));
    expect(x("acme-2")).toBeLessThan(x("acme-3"));
    expect(x("acme-3")).toBeLessThan(x("acme-4"));
    const fog = graph(pane).querySelector<HTMLElement>("[data-fog]")!;
    expect(Math.max(...["acme-2", "acme-3", "acme-4", "acme-5", "acme-6", "acme-7"].map(x))).toBeLessThan(Number(fog.getAttribute("x")));
  });

  it("shows Start only on frontier nodes, Draft and not blocked, and Start approves", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);

    const starts = within(graph(pane)).getAllByRole("button", { name: /^Start/ });
    expect(starts).toHaveLength(1);
    expect(within(node(pane, "acme-3")).getByRole("button", { name: "Start Which store" })).toBe(starts[0]);
    await userEvent.click(starts[0]);

    expect(fake.calls).toContain("approve");
    expect((await fake.getTicket("acme", "acme-3")).state).toBe("Ready");
    await waitFor(() => expect(node(pane, "acme-3")).toHaveTextContent("Ready"));
    expect(within(graph(pane)).queryByRole("button", { name: /^Start/ })).toBeNull();
  });

  it("opens a node's Ticket in the Ticket panel", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);

    await userEvent.click(within(node(pane, "acme-5")).getByRole("button", { name: /Ask legal/ }));
    expect(params().get("ticket")).toBe("acme-5");
    expect(await within(detail()).findByRole("heading", { name: "Ask legal" })).toBeInTheDocument();
  });

  it("draws one dim fog node on the right for Not yet specified, which shows that section when clicked", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);

    const fog = within(graph(pane)).getAllByRole("button", { name: "Not yet specified" });
    expect(fog).toHaveLength(1);
    expect(pane).not.toHaveTextContent("How conflicts merge.");
    await userEvent.click(fog[0]);
    expect(within(pane).getByRole("region", { name: "Not yet specified" })).toHaveTextContent("How conflicts merge.");
    expect(params().get("ticket")).toBeNull();
    await userEvent.click(fog[0]);
    expect(within(pane).queryByRole("region", { name: "Not yet specified" })).toBeNull();
  });

  it("heads the view with the Destination, and puts collapsible Decisions so far and Out of scope beside the graph", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);

    expect(within(pane).getByRole("heading", { name: "Offline mode", level: 2 })).toBeInTheDocument();
    expect(within(pane).getByLabelText("Destination")).toHaveTextContent("The app works on a plane.");
    const decisions = within(pane).getByText("Decisions so far", { selector: "summary" }).closest("details")!;
    const outOfScope = within(pane).getByText("Out of scope", { selector: "summary" }).closest("details")!;
    expect(decisions).toHaveTextContent("Store edits locally, from acme-2");
    expect(outOfScope).toHaveTextContent("Offline sign-up.");
    expect(decisions.open).toBe(true);
    await userEvent.click(within(decisions).getByText("Decisions so far"));
    expect(decisions.open).toBe(false);
    expect(outOfScope.open).toBe(true);
  });

  it("has a Guided toggle in its header that sets the guided Tag on and removes it", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);

    const guided = within(pane).getByRole("checkbox", { name: "Guided" });
    expect(guided).not.toBeChecked();
    await userEvent.click(guided);
    expect(fake.calls).toContain("setTags");
    expect((await fake.getSpec("acme", "acme-1")).tags).toEqual({ guided: "on" });
    await waitFor(() => expect(within(pane).getByRole("checkbox", { name: "Guided" })).toBeChecked());

    await userEvent.click(within(pane).getByRole("checkbox", { name: "Guided" }));
    expect(fake.calls).toContain("removeTags");
    expect((await fake.getSpec("acme", "acme-1")).tags).toEqual({});
    await waitFor(() => expect(within(pane).getByRole("checkbox", { name: "Guided" })).not.toBeChecked());
  });

  it("shows a Map as guided only when its guided Tag is on", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    await fake.setTags("acme", "acme-1", { guided: "yes" });
    const pane = await open(fake);

    expect(within(pane).getByRole("checkbox", { name: "Guided" })).not.toBeChecked();
  });

  it("says when a section is still empty", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitMap("acme", "Offline mode"); // acme-1, Charting acme-2
    const pane = await open(fake);

    expect(within(pane).getByLabelText("Destination")).toHaveTextContent("No Destination yet.");
    expect(within(pane).getByText("Decisions so far", { selector: "summary" }).closest("details")).toHaveTextContent("None yet.");
    await userEvent.click(within(graph(pane)).getByRole("button", { name: "Not yet specified" }));
    expect(within(pane).getByRole("region", { name: "Not yet specified" })).toHaveTextContent("Nothing written yet.");
  });
});

describe("Write the Spec", () => {
  const open = async (fake: FakeApi) => {
    renderApp(fake, "/?view=inbox&list=requirements&requirement=acme-1");
    return within(detail()).findByRole("region", { name: "Map" });
  };
  // A finished Map: every Ticket Done or Cancelled.
  const finished = (fake: FakeApi) =>
    aMap(fake, [
      { title: "Charting: Offline mode", label: "wayfinder:chart", state: "Done" }, // acme-2
      { title: "Which store", label: "wayfinder:grilling", state: "Done", blockedBy: ["acme-2"] }, // acme-3
      { title: "Dropped", label: "wayfinder:task", state: "Cancelled" }, // acme-4
    ]);

  it("isn't in the header while the Map has unfinished Tickets", async () => {
    const fake = new FakeApi();
    offlineMode(fake);
    const pane = await open(fake);
    expect(within(pane).queryByRole("button", { name: "Write the Spec" })).toBeNull();
  });

  it("opens the New Requirement dialog prefilled from the Map, submits it with the Map and selects the new Requirement", async () => {
    const fake = new FakeApi();
    finished(fake);
    const pane = await open(fake);

    await userEvent.click(within(pane).getByRole("button", { name: "Write the Spec" }));
    const modal = screen.getByRole("dialog", { name: "Write the Spec" });
    const text = within(modal).getByRole<HTMLTextAreaElement>("textbox", { name: "Requirement" });
    expect(text.value).toBe("Offline mode\n\nFrom the Map [acme-1](acme-1).\n\nThe app works on a plane.\n");
    expect(within(modal).queryByRole("combobox")).toBeNull();
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    expect(fake.calls).toContain("submitRequirement");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(params().get("requirement")).toBe("acme-5"));
    const spec = await fake.getSpec("acme", "acme-5");
    expect(spec.tags).toEqual({ map: "acme-1" });
    expect(spec.requirement?.map).toBe("acme-1");
    expect(spec.requirement?.stages.map((s) => s.name)).toEqual(["spec", "tickets"]);
    expect(await within(detail()).findByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it("links the Requirements made from the Map in its header", async () => {
    const fake = new FakeApi();
    finished(fake);
    await fake.submitRequirement("acme", "Offline mode", "acme-1"); // acme-5
    const pane = await open(fake);

    const made = within(pane).getByRole("link", { name: "acme-5" });
    expect(made.closest("p")).toHaveTextContent("Requirements from this Map: acme-5");
    await userEvent.click(made);
    expect(params().get("requirement")).toBe("acme-5");
    expect(await within(detail()).findByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it("shows a Requirement made from the Map with its grilling segment done, labelled Map and linking to the Map", async () => {
    const fake = new FakeApi();
    finished(fake);
    await fake.submitRequirement("acme", "Offline mode", "acme-1"); // acme-5, Stages acme-6 and acme-7
    renderApp(fake, "/?view=inbox&list=requirements&requirement=acme-5");

    const pane = await within(detail()).findByRole("region", { name: "Requirement" });
    const strip = within(pane).getByRole("list", { name: "Stages" });
    const segs = within(strip).getAllByRole("listitem");
    expect(segs.map((s) => s.textContent)).toEqual(["Map", "Spec", "Tickets", "Build"]);
    expect(segs[0].dataset.progress).toBe("done");
    expect(await within(pane).findByText("Waiting for a Factory to claim Spec. Nothing needs you yet.")).toBeInTheDocument();
    // In the list, the row's strip is a picture inside the row's link.
    expect(within(row("acme-5")).getByRole("list", { name: "Stages" })).toHaveTextContent("MapSpecTickets");

    await userEvent.click(within(segs[0]).getByRole("link", { name: "Map" }));
    expect(params().get("requirement")).toBe("acme-1");
    expect(await within(detail()).findByRole("region", { name: "Map" })).toBeInTheDocument();
  });
});
