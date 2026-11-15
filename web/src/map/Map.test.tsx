// Map tests: the real App on a WASM Core world. "New Map", the Map view and
// the Map's row in the Requirements view.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario, WORK, type Scenario } from "../api/scenario";
import { MAP_LABEL, type State } from "../api/types";

function renderApp(w: World, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

async function newProject() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  return { w, s };
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const list = () => screen.getByRole("main", { name: "Tickets" });
const params = () => new URLSearchParams(window.location.search);
const row = (id: string) => list().querySelector<HTMLElement>(`a[data-id="${id}"]`)!;

describe("New Map", () => {
  it("opens from the + New menu, submits the same free-text dialog and selects the new Map", async () => {
    const { w } = await newProject();
    renderApp(w);

    await userEvent.click(await within(nav()).findByRole("button", { name: "More to create" }));
    await userEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Map" }));
    const modal = screen.getByRole("dialog", { name: "New Map" });
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    expect(within(modal).getByRole("button", { name: "Submit map" })).toBeDisabled();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Map" }), "Offline mode{Enter}Works on a plane.");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit map" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const map = (await w.api.getSpec("acme", "acme-1"));
    expect(map.labels).toContain(MAP_LABEL);
    expect(map.requirement).toBeUndefined();
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
    const { w, s } = await newProject();
    const quiet = await s.api.submitMap("acme", "Offline mode");
    const busy = await s.api.submitMap("acme", "Sync");
    await s.accepted("acme", busy.charting.id);
    await s.human(`${s.ticketPath("acme", busy.charting.id)}/land`, { commit: WORK.commit });
    await s.ticket("acme", { title: "Which store", state: "In Review", spec: busy.spec.id, labels: ["wayfinder:grilling"] });
    await s.ticket("acme", { title: "Try a cache", state: "In Review", spec: busy.spec.id, labels: ["prototype"] });
    await s.ticket("acme", { title: "Dropped", state: "Cancelled", spec: busy.spec.id, labels: ["wayfinder:task"] });
    const req = await s.api.submitRequirement("acme", "Export a Spec");
    renderApp(w, "/?view=inbox&list=requirements");

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

interface MapTicket {
  title: string;
  label: string;
  state: State;
  blockedBy?: string[]; // titles of earlier Tickets
}

// aMap makes the Map with a body under the fixed headings and these
// Tickets, each with its Label, State and Blockers, and returns the Map's
// ID and the Tickets' IDs by title.
async function aMap(s: Scenario, tickets: MapTicket[]) {
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
  const { spec: map, charting } = await s.api.submitMap("acme", "Offline mode");
  await s.api.editSpec("acme", map.id, { body });
  const ids: Record<string, string> = {};
  for (const t of tickets) {
    if (t.label === "wayfinder:chart") {
      // submitMap made the Charting Ticket; the Map is charted when it is Done.
      ids[t.title] = charting.id;
      await s.accepted("acme", charting.id);
      await s.human(`${s.ticketPath("acme", charting.id)}/land`, { commit: WORK.commit });
      continue;
    }
    ids[t.title] = await s.ticket("acme", { title: t.title, spec: map.id, state: t.state, labels: [t.label] });
  }
  for (const t of tickets) {
    for (const b of t.blockedBy ?? []) await s.addBlocker("acme", ids[t.title], ids[b]);
  }
  return { map: map.id, ids };
}

// The Map below: Charting is done; Which store is the frontier and
// blocks Sync libraries; Ask legal runs; Try a cache needs the human;
// Pick a format is queued for a Factory.
const offlineMode = (s: Scenario) =>
  aMap(s, [
    { title: "Charting: Offline mode", label: "wayfinder:chart", state: "Done" },
    { title: "Which store", label: "wayfinder:grilling", state: "Draft", blockedBy: ["Charting: Offline mode"] },
    { title: "Sync libraries", label: "wayfinder:research", state: "Draft", blockedBy: ["Which store"] },
    { title: "Ask legal", label: "wayfinder:task", state: "In Progress", blockedBy: ["Charting: Offline mode"] },
    { title: "Try a cache", label: "prototype", state: "In Review" },
    { title: "Pick a format", label: "wayfinder:grilling", state: "Ready", blockedBy: ["Charting: Offline mode"] },
  ]);

describe("the Map view", () => {
  const open = async (w: World, map: string) => {
    renderApp(w, `/?view=inbox&list=requirements&requirement=${map}`);
    return within(detail()).findByRole("region", { name: "Map" });
  };
  const graph = (pane: HTMLElement) => within(pane).getByRole("list", { name: "Map Tickets" });
  const node = (pane: HTMLElement, id: string) => graph(pane).querySelector<HTMLElement>(`[data-id="${id}"]`)!;

  it("replaces the Requirement view with a left-to-right graph of the Map's Tickets, coloured by progress, with Blockers as edges", async () => {
    const { w, s } = await newProject();
    const { map, ids: t } = await offlineMode(s);
    const pane = await open(w, map);

    expect(within(detail()).queryByRole("region", { name: "Requirement" })).toBeNull();
    expect(within(detail()).queryByRole("list", { name: "Stages" })).toBeNull();
    expect(node(pane, t["Which store"])).toHaveTextContent("grilling");
    expect(node(pane, t["Which store"])).toHaveTextContent("Which store");
    expect(node(pane, t["Sync libraries"])).toHaveTextContent("research");
    expect(node(pane, t["Try a cache"])).toHaveTextContent("prototype");
    const progress = Object.fromEntries(Object.entries(t).map(([title, id]) => [title, node(pane, id).dataset.progress]));
    expect(progress).toEqual({
      "Charting: Offline mode": "done",
      "Which store": "waiting",
      "Sync libraries": "waiting",
      "Ask legal": "running",
      "Try a cache": "needs",
      "Pick a format": "queued",
    });

    const charting = t["Charting: Offline mode"];
    const edges = [...pane.querySelectorAll<SVGElement>("[data-edge]")].map((e) => e.dataset.edge).sort();
    expect(edges).toEqual(
      [`${charting}>${t["Which store"]}`, `${charting}>${t["Ask legal"]}`, `${charting}>${t["Pick a format"]}`, `${t["Which store"]}>${t["Sync libraries"]}`].sort(),
    );
    // Left to right: a Blocker sits left of what it blocks, and the fog node right of all.
    const x = (id: string) => Number(node(pane, id).getAttribute("x"));
    expect(x(charting)).toBeLessThan(x(t["Which store"]));
    expect(x(t["Which store"])).toBeLessThan(x(t["Sync libraries"]));
    const fog = graph(pane).querySelector<HTMLElement>("[data-fog]")!;
    expect(Math.max(...Object.values(t).map(x))).toBeLessThan(Number(fog.getAttribute("x")));
  });

  it("shows Start only on frontier nodes, Draft and not blocked, and Start approves", async () => {
    const { w, s } = await newProject();
    const { map, ids: t } = await offlineMode(s);
    const pane = await open(w, map);

    const starts = within(graph(pane)).getAllByRole("button", { name: /^Start/ });
    expect(starts).toHaveLength(1);
    expect(within(node(pane, t["Which store"])).getByRole("button", { name: "Start Which store" })).toBe(starts[0]);
    await userEvent.click(starts[0]);

    await waitFor(() => expect(node(pane, t["Which store"])).toHaveTextContent("Ready"));
    expect((await w.api.getTicket("acme", t["Which store"])).state).toBe("Ready");
    expect(within(graph(pane)).queryByRole("button", { name: /^Start/ })).toBeNull();
  });

  it("opens a node's Ticket in the Ticket panel", async () => {
    const { w, s } = await newProject();
    const { map, ids: t } = await offlineMode(s);
    const pane = await open(w, map);

    await userEvent.click(within(node(pane, t["Ask legal"])).getByRole("button", { name: /Ask legal/ }));
    expect(params().get("ticket")).toBe(t["Ask legal"]);
    expect(await within(detail()).findByRole("heading", { name: "Ask legal" })).toBeInTheDocument();
  });

  it("draws one dim fog node on the right for Not yet specified, which shows that section when clicked", async () => {
    const { w, s } = await newProject();
    const { map } = await offlineMode(s);
    const pane = await open(w, map);

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
    const { w, s } = await newProject();
    const { map } = await offlineMode(s);
    const pane = await open(w, map);

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
    const { w, s } = await newProject();
    const { map } = await offlineMode(s);
    const pane = await open(w, map);

    const guided = within(pane).getByRole("checkbox", { name: "Guided" });
    expect(guided).not.toBeChecked();
    await userEvent.click(guided);
    await waitFor(() => expect(within(pane).getByRole("checkbox", { name: "Guided" })).toBeChecked());
    expect((await w.api.getSpec("acme", map)).tags).toEqual({ guided: "on" });

    await userEvent.click(within(pane).getByRole("checkbox", { name: "Guided" }));
    await waitFor(() => expect(within(pane).getByRole("checkbox", { name: "Guided" })).not.toBeChecked());
    expect((await w.api.getSpec("acme", map)).tags).toEqual({});
  });

  it("shows a Map as guided only when its guided Tag is on", async () => {
    const { w, s } = await newProject();
    const { map } = await offlineMode(s);
    await w.api.setTags("acme", map, { guided: "yes" });
    const pane = await open(w, map);

    expect(within(pane).getByRole("checkbox", { name: "Guided" })).not.toBeChecked();
  });

  it("says when a section is still empty", async () => {
    const { w, s } = await newProject();
    const made = await s.api.submitMap("acme", "Offline mode");
    const pane = await open(w, made.spec.id);

    expect(within(pane).getByLabelText("Destination")).toHaveTextContent("No Destination yet.");
    expect(within(pane).getByText("Decisions so far", { selector: "summary" }).closest("details")).toHaveTextContent("None yet.");
    await userEvent.click(within(graph(pane)).getByRole("button", { name: "Not yet specified" }));
    expect(within(pane).getByRole("region", { name: "Not yet specified" })).toHaveTextContent("Nothing written yet.");
  });
});

describe("Write the Spec", () => {
  const open = async (w: World, id: string) => {
    renderApp(w, `/?view=inbox&list=requirements&requirement=${id}`);
    return within(detail()).findByRole("region", { name: "Map" });
  };
  // A finished Map: every Ticket Done or Cancelled.
  const finished = (s: Scenario) =>
    aMap(s, [
      { title: "Charting: Offline mode", label: "wayfinder:chart", state: "Done" },
      { title: "Which store", label: "wayfinder:grilling", state: "Done", blockedBy: ["Charting: Offline mode"] },
      { title: "Dropped", label: "wayfinder:task", state: "Cancelled" },
    ]);

  it("isn't in the header while the Map has unfinished Tickets", async () => {
    const { w, s } = await newProject();
    const { map } = await offlineMode(s);
    const pane = await open(w, map);
    expect(within(pane).queryByRole("button", { name: "Write the Spec" })).toBeNull();
  });

  it("opens the New Requirement dialog prefilled from the Map, submits it with the Map and selects the new Requirement", async () => {
    const { w, s } = await newProject();
    const { map } = await finished(s);
    const pane = await open(w, map);

    await userEvent.click(within(pane).getByRole("button", { name: "Write the Spec" }));
    const modal = screen.getByRole("dialog", { name: "Write the Spec" });
    const text = within(modal).getByRole<HTMLTextAreaElement>("textbox", { name: "Requirement" });
    expect(text.value).toBe(`Offline mode\n\nFrom the Map [${map}](${map}).\n\nThe app works on a plane.\n`);
    expect(within(modal).queryByRole("combobox")).toBeNull();
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    await waitFor(() => expect(params().get("requirement")).not.toBe(map));
    const made = params().get("requirement")!;
    const spec = await w.api.getSpec("acme", made);
    expect(spec.tags).toEqual({ map });
    expect(spec.requirement?.map).toBe(map);
    expect(spec.requirement?.stages.map((st) => st.name)).toEqual(["spec", "tickets"]);
    expect(await within(detail()).findByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it("links the Requirements made from the Map in its header", async () => {
    const { w, s } = await newProject();
    const { map } = await finished(s);
    const req = await s.api.submitRequirement("acme", "Offline mode", map);
    const pane = await open(w, map);

    const made = within(pane).getByRole("link", { name: req.spec.id });
    expect(made.closest("p")).toHaveTextContent(`Requirements from this Map: ${req.spec.id}`);
    await userEvent.click(made);
    expect(params().get("requirement")).toBe(req.spec.id);
    expect(await within(detail()).findByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it("shows a Requirement made from the Map with its grilling segment done, labelled Map and linking to the Map", async () => {
    const { w, s } = await newProject();
    const { map } = await finished(s);
    const req = await s.api.submitRequirement("acme", "Offline mode", map);
    renderApp(w, `/?view=inbox&list=requirements&requirement=${req.spec.id}`);

    const pane = await within(detail()).findByRole("region", { name: "Requirement" });
    const strip = within(pane).getByRole("list", { name: "Stages" });
    const segs = within(strip).getAllByRole("listitem");
    expect(segs.map((sg) => sg.textContent)).toEqual(["Map", "Spec", "Tickets", "Build"]);
    expect(segs[0].dataset.progress).toBe("done");
    expect(await within(pane).findByText("Waiting for a Factory to claim Spec. Nothing needs you yet.")).toBeInTheDocument();
    // In the list, the row's strip is a picture inside the row's link.
    expect(within(row(req.spec.id)).getByRole("list", { name: "Stages" })).toHaveTextContent("MapSpecTickets");

    await userEvent.click(within(segs[0]).getByRole("link", { name: "Map" }));
    expect(params().get("requirement")).toBe(map);
    expect(await within(detail()).findByRole("region", { name: "Map" })).toBeInTheDocument();
  });
});
