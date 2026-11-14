// Inbox layout tests: the real App and panels on a WASM Core world built
// with the scenario builder. They check layout only; what Core computes
// (group order, reasons) is rendered as received.
import { render, screen, within, waitFor, act, fireEvent, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { Api } from "../api/api";
import { newWorld, type World } from "../api/core/core";
import { scenario, type Scenario } from "../api/scenario";
import type { SubmitRequirementResult } from "../api/types";

const minutes = (m: number) => m * 60_000;

// startWorld is a fresh world with Projects keys, and its scenario.
async function startWorld(...keys: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  for (const k of keys) await s.project(k, k[0].toUpperCase() + k.slice(1));
  return { w, s };
}

// requirement submits a Requirement as the human: its Spec and Stage Tickets.
async function requirement(s: Scenario, key: string, text: string): Promise<SubmitRequirementResult> {
  return s.human(`${s.projectPath(key)}/submit_requirement`, { text });
}

// done takes a Ready Ticket to Done by hand.
async function done(s: Scenario, key: string, id: string) {
  await s.human(`${s.ticketPath(key, id)}/move`, { to_state: "Done" });
}

// doneStages takes every Stage Ticket of a Requirement to Done.
async function doneStages(s: Scenario, key: string, r: SubmitRequirementResult) {
  for (const t of r.stages) await done(s, key, t.id);
}

// world is Projects acme and beta. acme: acme-1 is its Spec, acme-2 Needs
// Answer (Round 1 with one of two Questions open, 5m ago), acme-3 Failed
// twice (90m ago), acme-4 a Draft, acme-5 Ready. beta: beta-1 is its Spec,
// beta-2 In Review (3h ago), beta-3 Ready, beta-4 a Draft.
async function world() {
  const { w, s } = await startWorld("acme", "beta");
  await s.ticket("acme", { title: "Pick a colour" }); // acme-2
  await s.ticket("acme", { title: "Fix login" }); // acme-3
  await s.ticket("acme", { title: "Rough idea", state: "Draft" }); // acme-4
  await s.ticket("acme", { title: "Acme ready" }); // acme-5
  await s.ticket("beta", { title: "Beta review" }); // beta-2
  await s.ticket("beta", { title: "Beta ready" }); // beta-3
  await s.ticket("beta", { title: "Beta sketch", state: "Draft" }); // beta-4
  await s.inReview("beta", "beta-2", undefined, "Added the login page");
  w.advance(minutes(90));
  await s.fail("acme", "acme-3", { ...REPORT_OF, what_broke: "Tests fail on CI" });
  await s.human(`${s.ticketPath("acme", "acme-3")}/send_back`, {});
  await s.fail("acme", "acme-3", { ...REPORT_OF, what_broke: "Tests fail on CI" });
  w.advance(minutes(85));
  await s.askRound("acme", "acme-2", [
    { title: "Which colour?", body: "Red or blue" },
    { title: "Which size?", body: "S or M" },
  ]);
  await s.human(`${s.ticketPath("acme", "acme-2")}/answer`, { question: 1, text: "Blue" });
  w.advance(minutes(5));
  return w;
}

const REPORT_OF = { what_broke: "", why: "The fixture server never starts", what_to_fix: "Start the fixture in TestMain", where_work_stands: "Branch impl, tests red" };

function renderApp(w: World, url = "/", api: Api = w.api) {
  window.history.replaceState(null, "", url);
  return render(<App api={api} now={w.now} />);
}

// cancelAll cancels the Tickets, as the human does to clear them from Needs Me.
async function cancelAll(w: World, key: string, ids: string[]) {
  const s = scenario(w);
  for (const id of ids) await s.human(`${s.ticketPath(key, id)}/cancel`, {});
}

const list = () => screen.getByRole("main", { name: "Tickets" });
const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const groupNames = () => within(list()).queryAllByRole("heading", { level: 3 }).map((h) => h.dataset.reason);
// Every row in the middle pane, collapsed Drafts included, in DOM order.
const rowIds = () => [...list().querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
const params = () => new URLSearchParams(window.location.search);

describe("the triage inbox", () => {
  it("renders Needs Me groups in the order the service returns, each row with its wait and detail", async () => {
    renderApp(await world());
    await waitFor(() => expect(groupNames()).toEqual(["needs_answer", "failed", "in_review"]));

    const row = within(list()).getByRole("link", { name: /acme-3/ });
    expect(row).toHaveTextContent("Fix login");
    expect(row).toHaveTextContent("1h");
    expect(row).toHaveTextContent("Tests fail on CI · failed 2×");
    expect(within(list()).getByRole("link", { name: /acme-2/ })).toHaveTextContent("1 of 2 open · Round 1");
  });

  it("keeps Drafts below in a collapsed group", async () => {
    renderApp(await world());
    const drafts = await within(list()).findByText(/Drafts \(2\)/);
    const group = drafts.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    expect(within(group).getByRole("link", { name: /acme-4/, hidden: true })).not.toBeVisible();
    // Drafts come after every Needs Me group.
    const sections = [...list().querySelectorAll("section, details")];
    expect(sections.at(-1)).toBe(group);
  });

  it("shows a State as a flat list with a dot on rows that need the human", async () => {
    // Core puts every Failed Ticket in Needs Me, so the row with no dot is a Ready one.
    renderApp(await world());
    await userEvent.click(await within(nav()).findByRole("button", { name: /^Failed/ }));

    await waitFor(() => expect(rowIds()).toEqual(["acme-3"]));
    expect(groupNames()).toEqual([]);
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toContainElement(screen.getByLabelText("Needs you"));
    expect(params().get("state")).toBe("Failed");

    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    await waitFor(() => expect(rowIds()).toEqual(["acme-5", "beta-3"]));
    expect(within(within(list()).getByRole("link", { name: /acme-5/ })).queryByLabelText("Needs you")).toBeNull();
  });

  it("lists Projects with their Needs Me counts and a View list with counts", async () => {
    renderApp(await world());
    await waitFor(() => expect(within(nav()).getByRole("button", { name: "Acme 2" })).toBeInTheDocument());
    expect(within(nav()).getByRole("button", { name: "Beta 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "All Projects 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Needs Me 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 2" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Done 0" })).toBeInTheDocument();
  });

  it("combines the Project and View filters", async () => {
    renderApp(await world());
    await userEvent.click(await within(nav()).findByRole("button", { name: /^Beta/ }));
    await waitFor(() => expect(rowIds()).toEqual(["beta-2", "beta-4"])); // Needs Me, then Drafts
    expect(within(nav()).getByRole("button", { name: "Needs Me 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 1" })).toBeInTheDocument();

    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    await waitFor(() => expect(rowIds()).toEqual(["beta-3"]));
    expect(params().get("project")).toBe("beta");
    expect(params().get("state")).toBe("Ready");

    await userEvent.click(within(nav()).getByRole("button", { name: /^All Projects/ }));
    await waitFor(() => expect(rowIds()).toEqual(["acme-5", "beta-3"]));
  });

  it("puts the view and the selected Ticket in the URL, and opens them from it", async () => {
    const w = await world();
    const { unmount } = renderApp(w);
    await userEvent.click(await within(list()).findByRole("link", { name: /acme-2/ }));
    expect(params().get("view")).toBe("inbox");
    expect(params().get("ticket")).toBe("acme-2");
    expect(await within(detail()).findByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
    unmount();

    renderApp(w, "/?view=inbox&state=Ready&ticket=beta-3");
    expect(await within(detail()).findByRole("heading", { name: "Beta ready" })).toBeInTheDocument();
    await waitFor(() => expect(rowIds()).toEqual(["acme-5", "beta-3"]));
    expect(within(list()).getByRole("link", { name: /beta-3/ })).toHaveAttribute("aria-current", "true");
  });

  it("shows the selected Ticket with its action panel slot first", async () => {
    renderApp(await world(), "/?view=inbox&ticket=acme-3");
    const heading = await within(detail()).findByRole("heading", { name: "Fix login" });
    const slot = within(detail()).getByRole("region", { name: "Action panel" });
    const body = within(detail()).getByRole("article", { name: "Ticket details" });
    expect(slot.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(heading.closest("aside")).toBe(detail());
    expect(within(detail()).getByText("Failed", { selector: ".state-chip" })).toBeInTheDocument();
  });

  it("refetches after each change notice", async () => {
    const w = await world();
    renderApp(w);
    await waitFor(() => expect(rowIds()).toContain("acme-2"));

    const id = await scenario(w).ticket("beta", { title: "Another sketch", state: "Draft" }); // sends ticket.changed
    await waitFor(() => expect(rowIds()).toContain(id));
    expect(within(list()).getByText(/Drafts \(3\)/)).toBeInTheDocument();
  });

  it("refetches Needs Me after a notice and everything after a reconnect", async () => {
    const w = await world();
    renderApp(w, "/?view=inbox&ticket=beta-3");
    await waitFor(() => expect(rowIds()).toContain("acme-2"));
    await within(detail()).findByRole("heading", { name: "Beta ready" });

    await cancelAll(w, "acme", ["acme-2", "acme-3"]);
    await cancelAll(w, "beta", ["beta-2"]);
    await waitFor(() => expect(groupNames()).toEqual([]));
    expect(screen.getByText("Nothing needs you.")).toBeInTheDocument();

    // A missed notice: the feed drops, the Ticket changes, the feed returns.
    w.calls.length = 0;
    w.dropStreams();
    const s = scenario(w);
    await s.human(`${s.ticketPath("beta", "beta-3")}/edit_ticket`, { title: "Beta renamed" });
    expect(await within(detail()).findByRole("heading", { name: "Beta renamed" })).toBeInTheDocument();
    expect(w.calls).toEqual(expect.arrayContaining(["listProjects", "needsMe", "findTickets", "getTicket"]));
  });

  it("shows the Needs Me count in the tab title and favicon", async () => {
    const w = await world();
    renderApp(w);
    await waitFor(() => expect(document.title).toBe("(3) Code Central"));
    const icon = () => document.querySelector<HTMLLinkElement>("link[rel=icon]")!;
    expect(decodeURIComponent(icon().href)).toContain(">3<");

    await cancelAll(w, "acme", ["acme-2", "acme-3"]);
    await cancelAll(w, "beta", ["beta-2"]);
    await waitFor(() => expect(document.title).toBe("Code Central"));
    expect(decodeURIComponent(icon().href)).not.toMatch(/>\d+</);
  });
});

// requirements is a Project with three Requirements and a plain Ticket:
// acme-1 at Spec, which needs an answer; acme-5 in Build, with two
// implementation Tickets needing the human; acme-11 waiting for a Factory
// to claim Grilling. acme-15 is a plain Spec, with acme-16 Failed on it.
async function requirements() {
  const { w, s } = await startWorld("acme");
  const first = await requirement(s, "acme", "Export a Spec as Markdown"); // acme-1, stages acme-2..4
  await done(s, "acme", first.stages[0].id);
  await s.askRound("acme", first.stages[1].id, [{ title: "Which format?", body: "CommonMark or GFM" }]);
  const building = await requirement(s, "acme", "Archive finished Projects"); // acme-5, stages acme-6..8
  await doneStages(s, "acme", building);
  const spec = building.spec.id;
  const review = await s.ticket("acme", { title: "Archive in the Project menu", spec }); // acme-9
  const failed = await s.ticket("acme", { title: "CLI: archive-project", spec }); // acme-10
  await requirement(s, "acme", "Render Markdown"); // acme-11, stages acme-12..14
  const plain = await s.spec("acme", { title: "Plain" }); // acme-15
  const lint = await s.ticket("acme", { title: "Plain work", spec: plain }); // acme-16
  await s.inReview("acme", review, undefined, "Menu item added");
  await s.fail("acme", failed, { ...REPORT_OF, what_broke: "Tests fail" });
  await s.fail("acme", lint, { ...REPORT_OF, what_broke: "Lint" });
  return w;
}

const groupHeads = () => within(list()).queryAllByRole("heading", { level: 3 }).map((h) => h.textContent);

describe("Requirements in Needs Me", () => {
  it("shows a Requirement whose Stage needs the human as one row in a Requirements group at the top, never the Stage Ticket", async () => {
    renderApp(await requirements());
    await waitFor(() => expect(groupHeads()).toEqual(["Requirements 1", "Failed 2", "In Review 1"]));
    expect(rowIds()).not.toContain("acme-3");
    const group = within(list()).getByRole("region", { name: "Requirements" });
    const row = within(group).getByRole("link", { name: /acme-1/ });
    expect(row).toHaveTextContent("Export a Spec as Markdown");
    expect(within(row).getByRole("list", { name: "Stages" })).toBeInTheDocument();
    expect(row).toHaveTextContent("Spec: Questions");
  });

  it("keeps Requirements not waiting on the human in a collapsed in-flight group", async () => {
    const w = await requirements();
    const s = scenario(w);
    await doneStages(s, "acme", await requirement(s, "acme", "Finished work")); // acme-17
    renderApp(w);
    const summary = await within(list()).findByText("Requirements in flight (1): not waiting on you");
    const group = summary.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    const row = within(group).getByRole("link", { name: /acme-11/, hidden: true });
    expect(row).not.toBeVisible();
    expect(row).toHaveTextContent("Grilling: waiting for a Factory");
    // Neither the Requirement in Build, whose Tickets show as rows, nor the finished one.
    expect([...group.querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id)).toEqual(["acme-11"]);
  });

  it("opens a Requirement row on the Stage that needs the human", async () => {
    renderApp(await requirements());
    await userEvent.click(await within(list()).findByRole("link", { name: /Export a Spec as Markdown/ }));
    expect(params().get("requirement")).toBe("acme-1");
    expect(within(detail()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
    expect(await within(detail()).findByRole("heading", { name: "Spec: Export a Spec as Markdown" })).toBeInTheDocument();
  });

  it("keeps implementation Tickets as ordinary rows naming their Requirement, several at once", async () => {
    renderApp(await requirements());
    const failed = await within(list()).findByRole("region", { name: "Failed" });
    expect(within(failed).getByRole("link", { name: /acme-10/ })).toHaveTextContent("↳ Archive finished Projects");
    expect(within(failed).getByRole("link", { name: /acme-16/ })).not.toHaveTextContent("↳");
    const review = within(list()).getByRole("region", { name: "In Review" });
    expect(within(review).getByRole("link", { name: /acme-9/ })).toHaveTextContent("↳ Archive finished Projects");
  });

  it("counts a Requirement row once in Needs Me, in place of its Stage Ticket", async () => {
    renderApp(await requirements());
    // acme-1's row, acme-9, acme-10 and acme-16.
    await waitFor(() => expect(within(nav()).getByRole("button", { name: "Needs Me 4" })).toBeInTheDocument());
  });

  it("counts Requirement rows, not Stage Tickets, in the tab title and All Projects too", async () => {
    const w = await requirements();
    // Core runs a Requirement's Stages one at a time, so only one Stage Ticket
    // of acme-1 can need the human; it is one row.
    renderApp(w);
    await waitFor(() => expect(within(nav()).getByRole("button", { name: "Needs Me 4" })).toBeInTheDocument());
    expect(within(nav()).getByRole("button", { name: "All Projects 4" })).toBeInTheDocument();
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
  });
});

describe("Stage Tickets in State views", () => {
  it("folds them into a collapsed group and leaves them out of the State counts", async () => {
    renderApp(await requirements());
    // Ready: acme-4 and acme-12 are Stage Tickets; nothing else is Ready.
    await userEvent.click(await within(nav()).findByRole("button", { name: "Ready 0" }));
    expect(within(nav()).getByRole("button", { name: "Done 0" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Failed 2" })).toBeInTheDocument();
    const summary = within(list()).getByText("Stage Tickets (4): run by a Factory");
    const group = summary.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    expect(within(group).getByRole("link", { name: /acme-4/, hidden: true })).not.toBeVisible();
    expect(rowIds()).toEqual(["acme-4", "acme-12", "acme-13", "acme-14"]);
  });

  it("lists other Tickets as before, the fold after them", async () => {
    const w = await requirements();
    const s = scenario(w);
    await s.ticket("acme", { title: "Shipped", spec: "acme-15", state: "Done" }); // acme-17
    renderApp(w, "/?view=inbox&state=Done");
    // Besides acme-17, acme-2 and the three Stages of acme-5 are Done.
    await waitFor(() => expect(within(list()).getByText("Stage Tickets (4): run by a Factory")).toBeInTheDocument());
    expect(within(nav()).getByRole("button", { name: "Done 1" })).toBeInTheDocument();
    expect(rowIds()[0]).toBe("acme-17");
    expect(rowIds()).toHaveLength(5);
    expect(within(list()).getByRole("link", { name: /acme-17/ })).toBeVisible();
  });
});

// inProgress is two In Progress Tickets with live Claims, and a Needs
// Answer one whose live Claim has a latest event too.
async function inProgress() {
  const { w, s } = await startWorld("acme");
  const busy = await s.ticket("acme", { title: "Busy work" }); // acme-2, on the Project's Spec acme-1
  const quiet = await s.ticket("acme", { title: "Quiet work" }); // acme-3
  const asking = await s.ticket("acme", { title: "Asking work" }); // acme-4
  await s.factorySession("acme", busy, {
    session: "s-busy",
    events: [{ time: w.now().toISOString(), kind: "tool_call", tool: "Read", text: "Read: core/core.go" }],
  });
  await s.factorySession("acme", quiet, { session: "s-quiet" });
  await s.factorySession("acme", asking, { session: "s-asking" });
  await s.askRound("acme", asking);
  await s.postSessionEvents("s-asking", [{ time: w.now().toISOString(), kind: "factory", text: "acme-4 posted a Round" }]);
  w.advance(minutes(2));
  return { w, s };
}

describe("the latest Session Event in the In Progress list", () => {
  it("shows each row's latest event faintly under its title, and nothing on a row with none", async () => {
    renderApp((await inProgress()).w, "/?view=inbox&state=In%20Progress");
    const busy = await within(list()).findByRole("link", { name: /acme-2/ });
    const line = within(busy).getByText("Read: core/core.go · 2m ago");
    expect(line).toHaveClass("row-detail", "latest-event");
    const quiet = within(list()).getByRole("link", { name: /acme-3/ });
    expect(quiet.textContent).toBe("acme-3Quiet work");
  });

  it("updates on the live feed without a reload", async () => {
    const { w, s } = await inProgress();
    renderApp(w, "/?view=inbox&state=In%20Progress");
    await within(list()).findByText("Read: core/core.go · 2m ago");
    await act(() => s.postSessionEvents("s-quiet", [{ time: w.now().toISOString(), kind: "text", text: "Writing the test first." }]));
    const quiet = within(list()).getByRole("link", { name: /acme-3/ });
    expect(await within(quiet).findByText("Writing the test first. · just now")).toBeInTheDocument();
  });

  it("leaves it off rows in other States", async () => {
    renderApp((await inProgress()).w, "/?view=inbox&state=Needs%20Answer");
    const asking = await within(list()).findByRole("link", { name: /acme-4/ });
    expect(asking).not.toHaveTextContent("posted a Round");
  });
});

// wayfinding is a Map acme-1 whose Charting Ticket acme-2 needs an answer
// and whose Prototype acme-3 is In Review, and a Prototype acme-5 on the
// plain Spec acme-4, also In Review.
async function wayfinding() {
  const { w, s } = await startWorld("acme");
  const map = await s.human(`${s.projectPath("acme")}/submit_map`, { text: "Find the way" }); // acme-1, Charting Ticket acme-2
  await s.askRound("acme", map.charting.id);
  const prototype = async (spec: string, text: string) => (await s.human(`${s.projectPath("acme")}/submit_prototype`, { spec, text })).id;
  const graph = await prototype(map.spec.id, "Prototype: the graph"); // acme-3
  const plain = await s.spec("acme", { title: "Login" }); // acme-4
  const form = await prototype(plain, "Prototype: the form"); // acme-5
  await s.inReview("acme", graph);
  await s.inReview("acme", form);
  return w;
}

describe("Map Tickets and Prototypes in Needs Me", () => {
  it("lists each as its own row with its Map's or Spec's name, and a Prototype badge on a Prototype", async () => {
    renderApp(await wayfinding());
    await waitFor(() => expect(rowIds()).toEqual(["acme-2", "acme-3", "acme-5"]));
    expect(groupHeads()).toEqual(["Questions 1", "In Review 2"]);
    const row = (id: string) => within(list()).getByRole("link", { name: new RegExp(id) });
    expect(row("acme-2")).toHaveTextContent("↳ Map: Find the way");
    expect(within(row("acme-2")).queryByText("Prototype")).toBeNull();
    expect(row("acme-3")).toHaveTextContent("↳ Map: Find the way");
    expect(within(row("acme-3")).getByText("Prototype")).toHaveClass("label", "prototype-badge");
    expect(row("acme-5")).toHaveTextContent("↳ Login");
    expect(row("acme-5")).not.toHaveTextContent("Map:");
    expect(within(row("acme-5")).getByText("Prototype")).toHaveClass("prototype-badge");
  });
});

describe("the sidebar's top row: + New and Focus", () => {
  const menuButton = () => within(nav()).getByRole("button", { name: "More to create" });
  const findMenuButton = () => within(nav()).findByRole("button", { name: "More to create" });
  const item = (name: string) => within(screen.getByRole("menu")).getByRole("menuitem", { name });

  it("shows + New, its ▾ and Focus at the top of the sidebar", async () => {
    renderApp(await world());
    const newButton = await within(nav()).findByRole("button", { name: "New requirement" });
    expect(newButton).toHaveTextContent("+ New");
    expect(menuButton()).toHaveAttribute("aria-haspopup", "menu");
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(within(nav()).getByRole("button", { name: "Focus" })).toHaveTextContent("◎ Focus");
    expect(within(nav()).queryByRole("button", { name: "New Map" })).toBeNull();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("shows + New and its ▾ as primary", async () => {
    renderApp(await world());
    expect(await within(nav()).findByRole("button", { name: "New requirement" })).toHaveClass("primary");
    expect(menuButton()).toHaveClass("primary");
  });

  it("opens the New requirement dialog from + New", async () => {
    renderApp(await world());
    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens a menu of Requirement, Map and Small Change from ▾, each with its hint", async () => {
    renderApp(await world());
    await userEvent.click(await findMenuButton());
    expect(menuButton()).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("menu", { name: "More to create" })).toBeVisible();
    const items = within(screen.getByRole("menu")).getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual([
      expect.stringContaining("Requirement"),
      expect.stringContaining("Map"),
      expect.stringContaining("Small Change"),
    ]);
    expect(item("Requirement")).toHaveAccessibleDescription(
      "Grilled into a Spec and Tickets",
    );
    expect(item("Map")).toHaveAccessibleDescription(
      "Charts the decisions to get somewhere",
    );
    expect(item("Small Change")).toHaveAccessibleDescription("One Ticket, built and landed as it is");
  });

  it("opens Small Change's dialog from the menu, and submits to the new Ticket", async () => {
    const w = await world();
    renderApp(w, "/?project=acme");
    await userEvent.click(await findMenuButton());
    await userEvent.click(item("Small Change"));

    expect(screen.queryByRole("menu")).toBeNull();
    const modal = screen.getByRole("dialog", { name: "New Small Change" });
    const box = within(modal).getByRole("textbox", { name: "Small Change" });
    expect(box).toHaveAttribute("placeholder", "Describe the change you want. The first line becomes the Ticket's title.");
    expect(within(modal).getByText("Creates a Ready Ticket on the Small Changes Spec. A Factory builds it and it lands on its own.")).toBeVisible();
    const submit = within(modal).getByRole("button", { name: "Submit small change" });
    expect(submit).toBeDisabled();
    await userEvent.type(box, "   ");
    expect(submit).toBeDisabled();
    await userEvent.type(box, "Rename the button");
    await userEvent.click(submit);

    expect(w.calls).toContain("submitSmallChange");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const created = (await w.api.findTickets("acme")).find((t) => t.title === "Rename the button")!;
    expect(params().get("ticket")).toBe(created.id);
    expect(params().get("requirement")).toBeNull();
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("asks which Project for a Small Change when several are open and none is filtered", async () => {
    renderApp(await world());
    await userEvent.click(await findMenuButton());
    await userEvent.click(item("Small Change"));
    expect(within(screen.getByRole("dialog")).getByRole("combobox")).toBeVisible();
  });

  it("keeps the text and shows the error when a Small Change is refused", async () => {
    // Core would take this, so the refusal is the service's, injected.
    const w = await world();
    const refusing: Api = {
      ...w.api,
      submitSmallChange: async () => {
        throw new Error("the Project is archived");
      },
    };
    renderApp(w, "/?project=acme", refusing);
    await userEvent.click(await findMenuButton());
    await userEvent.click(item("Small Change"));
    const modal = screen.getByRole("dialog");
    await userEvent.type(within(modal).getByRole("textbox"), "Rename the button");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit small change" }));
    expect(await within(modal).findByText("the Project is archived")).toBeVisible();
    expect(within(modal).getByRole("textbox")).toHaveValue("Rename the button");
  });

  it.each([
    ["Requirement", "New requirement", "Submit requirement", "submitRequirement"],
    ["Map", "New Map", "Submit map", "submitMap"],
  ])("opens %s's dialog from the menu, which stays visible once the menu closes, and submits to the new Spec", async (name, dialog, submit, call) => {
    const w = await world();
    renderApp(w, "/?project=acme");
    await userEvent.click(await findMenuButton());
    await userEvent.click(item(name));

    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    const modal = screen.getByRole("dialog", { name: dialog });
    expect(modal).toBeVisible();
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await userEvent.click(within(modal).getByRole("button", { name: submit }));

    expect(w.calls).toContain(call);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(params().get("list")).toBe("requirements");
    expect(params().get("requirement")).toBe("acme-6");
    // The note is Focus Mode's: the sidebar navigates instead.
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("closes the menu on Esc and on a click outside it", async () => {
    renderApp(await world());
    await userEvent.click(await findMenuButton());
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");

    await userEvent.click(menuButton());
    expect(screen.getByRole("menu")).toBeInTheDocument();
    await userEvent.click(list());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("closes the menu when + New is clicked instead", async () => {
    renderApp(await world());
    await userEvent.click(await findMenuButton());
    await userEvent.click(within(nav()).getByRole("button", { name: "New requirement" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
  });

  it("toggles the menu shut from ▾", async () => {
    renderApp(await world());
    await userEvent.click(await findMenuButton());
    await userEvent.click(menuButton());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  describe("by keyboard", () => {
    const focusMenuButton = async () => (await findMenuButton()).focus();

    it("opens with ↓ on the first item, and moves with the arrows, wrapping at either end", async () => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      expect(menuButton()).toHaveAttribute("aria-expanded", "true");
      expect(item("Requirement")).toHaveFocus();

      await userEvent.keyboard("{ArrowDown}");
      expect(item("Map")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Small Change")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Requirement")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Small Change")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Map")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("opens with %s on the first item", async (key) => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard(key);
      expect(item("Requirement")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("picks the focused item with %s: the menu closes and its dialog opens", async (key) => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}");
      await userEvent.keyboard(key);
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(screen.getByRole("dialog", { name: "New Map" })).toBeVisible();
    });

    it("leaves the arrows to the dialog an item opened", async () => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Enter}");
      const text = within(screen.getByRole("dialog", { name: "New Map" })).getByRole("textbox");
      await userEvent.type(text, "Offline{Enter}mode");
      // fireEvent is false when a handler cancelled the key, as the caret would stay put.
      expect(fireEvent.keyDown(text, { key: "ArrowUp" })).toBe(true);
      expect(fireEvent.keyDown(text, { key: "ArrowDown" })).toBe(true);
      expect(text).toHaveFocus();
    });

    it("closes on Esc and returns focus to ▾", async () => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      renderApp(await world());
      await focusMenuButton();
      await userEvent.keyboard("{ArrowDown}");
      await userEvent.tab();
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    });
  });

  it("enters Focus Mode from Focus", async () => {
    renderApp(await world());
    await userEvent.click(await within(nav()).findByRole("button", { name: "Focus" }));
    expect(params().get("view")).toBe("focus");
  });
});

describe("the Admin section", () => {
  const follows = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_PRECEDING) !== 0;

  it("keeps the title row to the heading, with no Usage or Options links", async () => {
    renderApp(await world());
    const title = within(nav()).getByRole("heading", { level: 1, name: "Code Central" });
    expect(within(title.parentElement!).queryByRole("link")).toBeNull();
  });

  it("follows View's last State filter and precedes the Archived Projects", async () => {
    const w = await world();
    await scenario(w).project("old", "Old");
    await w.api.archiveProject("old");
    renderApp(w);
    const admin = within(nav()).getByRole("heading", { level: 2, name: "Admin" });
    const lastState = within(nav()).getByRole("button", { name: /^Cancelled/ });
    const archived = await within(nav()).findByText(/^Archived/);
    expect(follows(admin, lastState)).toBe(true);
    expect(follows(archived, admin)).toBe(true);

    const ci = within(nav()).getByRole("link", { name: "CI" });
    const usage = within(nav()).getByRole("link", { name: "Usage" });
    const options = within(nav()).getByRole("link", { name: "Options" });
    expect(follows(ci, admin)).toBe(true);
    expect(follows(usage, ci)).toBe(true);
    expect(follows(options, usage)).toBe(true);
    expect(follows(archived, options)).toBe(true);
  });

  it("links Usage and Options with real addresses that return to the Inbox", async () => {
    renderApp(await world(), "/?view=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).toHaveAttribute("href", "?view=usage&back=view%3Dinbox%26project%3Dacme%26state%3DReady");
    expect(within(nav()).getByRole("link", { name: "Options" })).toHaveAttribute("href", "?view=options&from=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).not.toHaveAttribute("aria-current");
  });

  it("opens Usage and Options", async () => {
    renderApp(await world());
    await userEvent.click(within(nav()).getByRole("link", { name: "Usage" }));
    expect(screen.getByRole("main", { name: "Usage" })).toBeInTheDocument();

    act(() => window.history.back());
    await userEvent.click(within(await screen.findByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Options" }));
    expect(screen.getByRole("main", { name: "Options" })).toBeInTheDocument();
  });
});

// bySpecWorld seeds Tickets on a Requirement (acme-1, with its Stages
// acme-2..4), a plain Spec and a Map, across States.
async function bySpecWorld() {
  const { w, s } = await startWorld("acme", "beta");
  const req = await requirement(s, "acme", "Export a Spec as Markdown"); // acme-1, Stages acme-2..4 (Ready)
  await done(s, "acme", req.stages[0].id);
  const plain = await s.spec("acme", { title: "Archive finished Projects" }); // acme-5
  await s.ticket("acme", { title: "Archive menu", spec: plain }); // acme-6
  const cli = await s.ticket("acme", { title: "Archive CLI", spec: plain }); // acme-7
  await s.ticket("acme", { title: "Archive docs", spec: plain }); // acme-8
  await s.human(`${s.projectPath("acme")}/submit_map`, { text: "Wayfinding" }); // acme-9, Charting Ticket acme-10 (Ready)
  await s.ticket("acme", { title: "Export button", spec: req.spec.id }); // acme-11
  const other = await s.spec("beta", { title: "Beta plan" }); // beta-1
  await s.ticket("beta", { title: "Beta thing", spec: other }); // beta-2
  await s.fail("acme", cli, { ...REPORT_OF, what_broke: "Tests fail" });
  return w;
}

const specRows = () => within(list()).queryAllByRole("region");
const specRow = (name: string) => within(list()).getByRole("region", { name });
// The Tickets an open Spec row lists; none while it is closed.
const openIds = (name: string) => {
  const tickets = within(specRow(name)).queryByRole("group", { name: `Tickets of ${name}` });
  return tickets ? within(tickets).getAllByRole("link").map((a) => a.dataset.id) : [];
};
// The Spec row's own link, the one that opens the Spec.
const specLinkOf = (name: string) => specRow(name).querySelector<HTMLElement>(`a[data-id]`)!;
const bySpec = () => userEvent.click(within(list()).getByRole("button", { name: "Spec" }));

describe("State lists By Spec", () => {
  beforeEach(() => localStorage.clear());

  it("shows the control on State lists only, with By Ticket the default and today's rows", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    const byTicket = await within(list()).findByRole("button", { name: "Ticket" });
    expect(byTicket).toHaveAttribute("aria-pressed", "true");
    expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "false");
    expect(rowIds()).toContain("acme-6");

    await userEvent.click(within(nav()).getByRole("button", { name: /^Needs Me/ }));
    expect(within(list()).queryByRole("button", { name: "Spec" })).toBeNull();
    await userEvent.click(within(nav()).getByRole("button", { name: /^Requirements/ }));
    expect(within(list()).queryByRole("button", { name: "Spec" })).toBeNull();
  });

  it("groups into collapsed Spec rows in first-Ticket order, with kind, ID, n of m and chips", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    await within(list()).findByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    expect(specRows().map((c) => c.getAttribute("aria-label"))).toEqual([
      "Export a Spec as Markdown", // its Stage acme-3 comes first
      "Archive finished Projects",
      "Wayfinding",
      "Beta plan",
    ]);
    expect(rowIds()).toEqual(["acme-1", "acme-5", "acme-9", "beta-1"]); // collapsed: the Spec rows alone

    const archive = specRow("Archive finished Projects");
    expect(within(archive).getByText("Spec")).toBeInTheDocument();
    expect(archive).toHaveTextContent("acme-5");
    expect(archive).toHaveTextContent("2 of 3");
    expect(within(archive).getByText("Ready 2")).toHaveAttribute("aria-current", "true");
    expect(within(archive).getByText("Failed 1")).not.toHaveAttribute("aria-current");
    expect(within(specRow("Wayfinding")).getByText("Map")).toBeInTheDocument();
    // A Requirement shows its Stage strip in place of chips.
    const req = specRow("Export a Spec as Markdown");
    expect(within(req).getByRole("list", { name: "Stages" })).toBeInTheDocument();
    expect(within(req).queryByText("Done 1")).toBeNull();
    expect(req).toHaveTextContent("3 of 4");
  });

  it("opens a Spec row with its Tickets in flat-list order, Stage Tickets marked, no Stage group", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    await userEvent.click(await within(list()).findByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-3", "acme-4", "acme-11"]);
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toHaveTextContent("spec");
    expect(within(list()).queryByText(/run by a Factory/)).toBeNull();
    expect(params().has("requirement")).toBe(false);
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual([]);
  });

  it("opens the selected Ticket's Spec row on its own, which can be closed", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec&ticket=acme-8");
    await waitFor(() => expect(openIds("Archive finished Projects")).toEqual(["acme-6", "acme-8"]));
    await userEvent.click(within(list()).getByRole("button", { name: "Archive finished Projects" }));
    expect(openIds("Archive finished Projects")).toEqual([]);
  });

  it("opens the Spec from a click on its row", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec&ticket=acme-8");
    await within(list()).findByRole("region", { name: "Wayfinding" });
    await userEvent.click(specLinkOf("Wayfinding"));
    expect(params().get("requirement")).toBe("acme-9");
    expect(params().get("ticket")).toBeNull();
    expect(params().get("group")).toBe("spec");
    expect(specLinkOf("Wayfinding")).toHaveAttribute("aria-current", "true");
  });

  it("marks a Spec row whose Ticket needs the human", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Failed&group=spec");
    const archive = await within(list()).findByRole("region", { name: "Archive finished Projects" });
    expect(within(archive).getByLabelText("Needs you")).toBeInTheDocument();
    await userEvent.click(within(archive).getByRole("button", { name: "Archive finished Projects" }));
    expect(within(within(list()).getByRole("link", { name: /acme-7/ })).getByLabelText("Needs you")).toBeInTheDocument();
  });

  it("keeps the grouping across lists and Projects, and from the address", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    await within(list()).findByRole("button", { name: "Archive finished Projects" });
    await userEvent.click(within(nav()).getByRole("button", { name: /^Failed/ }));
    expect(params().get("group")).toBe("spec");
    expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Needs Me/ }));
    expect(params().get("group")).toBe("spec");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Acme/ }));
    expect(params().get("group")).toBe("spec");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    expect(specRows().map((c) => c.getAttribute("aria-label"))).not.toContain("Beta plan");
  });

  it("uses the last pick when the address has none, and By Ticket clears it", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    await within(list()).findByRole("button", { name: "Spec" });
    await bySpec();
    expect(localStorage.getItem("code-central.inbox.group")).toBe("spec");

    window.history.replaceState(null, "", "/?view=inbox&state=Ready");
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    await waitFor(() => expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true"));

    await userEvent.click(within(list()).getByRole("button", { name: "Ticket" }));
    expect(params().has("group")).toBe(false);
    expect(localStorage.getItem("code-central.inbox.group")).toBe("ticket");
    expect(rowIds()).toContain("acme-6");
  });

  it("falls back to By Ticket when storage throws", async () => {
    const get = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    await within(list()).findByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    get.mockRestore();
    set.mockRestore();
  });

  it("says so when a State has no Tickets", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Cancelled&group=spec");
    expect(await within(list()).findByText("No Tickets Cancelled.")).toBeInTheDocument();
  });

  it("renders only Spec rows for a long collapsed list, newest first inside a Done Spec row", async () => {
    const { w, s } = await startWorld("acme");
    const specs = [];
    for (const t of ["A", "B", "C"]) specs.push(await s.spec("acme", { title: `Spec ${t}` }));
    for (let i = 0; i < 60; i++) {
      const id = await s.ticket("acme", { title: `Done ${i}`, spec: specs[i % 3] });
      await done(s, "acme", id);
      w.advance(1000);
    }
    renderApp(w, "/?view=inbox&state=Done&group=spec");
    await waitFor(() => expect(specRows()).toHaveLength(3));
    expect(rowIds()).toHaveLength(3);
    // The newest Ticket (i=59, Spec C) is first, so its Spec row leads.
    expect(specRows()[0]).toHaveAttribute("aria-label", "Spec C");
    await userEvent.click(within(specRows()[0]).getByRole("button", { name: "Spec C" }));
    // Newest first inside the Spec row: Done 59, 56, 53 ...
    const tickets = within(specRows()[0]).getByRole("group", { name: "Tickets of Spec C" });
    expect(within(tickets).getAllByRole("link").slice(0, 2).map((a) => a.textContent)).toEqual([
      expect.stringContaining("Done 59"),
      expect.stringContaining("Done 56"),
    ]);
    expect(rowIds()).toHaveLength(23);
  });

  it("loads every Spec with one findSpecs call per Project", async () => {
    const w = await bySpecWorld();
    w.calls.length = 0;
    renderApp(w);
    await within(nav()).findByRole("button", { name: /^Ready/ });
    expect(w.calls.filter((c) => c === "findSpecs")).toHaveLength(2);
  });
});

describe("the Spec row", () => {
  beforeEach(() => localStorage.clear());

  // withLongLived adds a Long-lived Spec (acme-12) with a Ready Ticket
  // (acme-13) and one a Factory is on (acme-14).
  async function withLongLived(w: World) {
    const s = scenario(w);
    const spec = await s.spec("acme", { title: "CI Fixes" }); // acme-12
    await w.api.addLabels("acme", spec, ["long-lived"]);
    await s.ticket("acme", { title: "Fix the flaky test", spec }); // acme-13
    await s.ticket("acme", { title: "Fix lint", spec, state: "In Progress" }); // acme-14
    return w;
  }

  it("badges every kind but a Requirement, and shows the strip for a Requirement and State chips for the rest", async () => {
    renderApp(await withLongLived(await bySpecWorld()), "/?view=inbox&state=Ready&group=spec");
    const req = await within(list()).findByRole("region", { name: "Export a Spec as Markdown" });
    expect(within(req).getByRole("list", { name: "Stages" })).toBeInTheDocument();
    expect(within(req).queryByText("Requirement")).toBeNull();
    expect(within(req).queryByText(/^Ready \d+$/)).toBeNull();

    for (const [name, kind] of [["Archive finished Projects", "Spec"], ["Wayfinding", "Map"], ["CI Fixes", "Long-lived"]]) {
      expect(within(specRow(name)).getByText(kind)).toBeInTheDocument();
      expect(within(specRow(name)).queryByRole("list", { name: "Stages" })).toBeNull();
      expect(within(specRow(name)).getByText(/^Ready \d+$/)).toHaveAttribute("aria-current", "true");
    }
    expect(within(specRow("CI Fixes")).getByText("In Progress 1")).not.toHaveAttribute("aria-current");
  });

  it("heads a plain Spec with its first need, else a Factory on it, else its Done count alone", async () => {
    renderApp(await withLongLived(await bySpecWorld()), "/?view=inbox&state=Ready&group=spec");
    const archive = await within(list()).findByRole("region", { name: "Archive finished Projects" });
    expect(archive).toHaveTextContent("Failed · 0 of 3 Done");
    expect(specRow("CI Fixes")).toHaveTextContent("A Factory is on it · 0 of 2 Done");
    expect(specRow("CI Fixes")).toHaveTextContent("1 of 2");
    const beta = specRow("Beta plan");
    expect(beta).toHaveTextContent("0 of 1 Done");
    expect(beta).not.toHaveTextContent("·");
    expect(beta).not.toHaveTextContent("1 of 1");
  });

  it("heads a finished Spec with its Done count", async () => {
    const { w, s } = await startWorld("acme");
    const spec = await s.spec("acme", { title: "Shipped" }); // acme-1
    for (const state of ["Done", "Done", "Cancelled"] as const) await s.ticket("acme", { title: state, state, spec });
    renderApp(w, "/?view=inbox&state=Done&group=spec");
    const row = await within(list()).findByRole("region", { name: "Shipped" });
    expect(row).toHaveTextContent("Finished · 2 of 3 Tickets Done");
    expect(row).toHaveTextContent("2 of 3");
  });

  it("dots a Spec that needs the human, and no other", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    const archive = await within(list()).findByRole("region", { name: "Archive finished Projects" });
    // acme-7 needs the human, though it isn't in the Ready list.
    expect(within(archive).getByLabelText("Needs you")).toBeInTheDocument();
    for (const name of ["Export a Spec as Markdown", "Wayfinding", "Beta plan"]) expect(within(specRow(name)).queryByLabelText("Needs you")).toBeNull();
  });

  it("expands a Requirement in the Requirements view to all its Tickets, Stages first, with State chips, without navigating", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&list=requirements");
    await userEvent.click(await within(list()).findByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-2", "acme-3", "acme-4", "acme-11"]);
    expect(within(list()).getByRole("link", { name: /acme-2/ })).toHaveTextContent("Done");
    expect(within(list()).getByRole("link", { name: /acme-11/ })).toHaveTextContent("Ready");
    expect(params().has("requirement")).toBe(false);
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual([]);
  });

  it("shows a Map in the Requirements view with chips and its decision count", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&list=requirements");
    const map = await within(list()).findByRole("region", { name: "Wayfinding" });
    expect(within(map).getByText("Map")).toBeInTheDocument();
    expect(within(map).getByText("Ready 1")).toBeInTheDocument();
    expect(map).toHaveTextContent("0/1 decided"); // Core counts the Charting Ticket
    await userEvent.click(within(map).getByRole("button", { name: "Wayfinding" }));
    expect(openIds("Wayfinding")).toEqual(["acme-10"]);
  });

  it("opens the selected Ticket's row on its own in the Requirements view, and a row click opens the Spec", async () => {
    renderApp(await bySpecWorld(), "/?view=inbox&list=requirements&ticket=acme-11");
    await waitFor(() => expect(openIds("Export a Spec as Markdown")).toContain("acme-11"));
    await userEvent.click(specLinkOf("Export a Spec as Markdown"));
    expect(params().get("requirement")).toBe("acme-1");
    expect(specLinkOf("Export a Spec as Markdown")).toHaveAttribute("aria-current", "true");
  });

  it("expands a Requirement in Needs Me to its Tickets", async () => {
    renderApp(await requirements());
    const group = await within(list()).findByRole("region", { name: "Requirements" });
    await userEvent.click(within(group).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-2", "acme-3", "acme-4"]);
    expect(within(within(list()).getByRole("link", { name: /acme-3/ })).getByLabelText("Needs you")).toBeInTheDocument();
    expect(params().has("requirement")).toBe(false);
  });
});

describe("Short IDs", () => {
  const idIn = (name: string) => within(list()).findByRole("button", { name });

  it("shows #n with one Project selected and the full ID with All Projects, the full ID on hover", async () => {
    renderApp(await world(), "/?state=Ready&project=acme");
    const short = await idIn("acme-5");
    expect(short).toHaveTextContent("#5");
    expect(short).toHaveAttribute("title", "acme-5");
    cleanup();
    renderApp(await world(), "/?state=Ready");
    expect(await idIn("acme-5")).toHaveTextContent("acme-5");
  });

  it("copies the full ID on click, flashes Copied and leaves the URL alone", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = await idIn("acme-5");
    await user.click(id);
    expect(writeText).toHaveBeenCalledWith("acme-5");
    expect(await within(list()).findByText("Copied")).toBeInTheDocument();
    expect(window.location.href).toBe(before);
  });

  it("does nothing when there is no clipboard", async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
    renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = await idIn("acme-5");
    await user.click(id);
    expect(id).toHaveTextContent("#5");
    expect(window.location.href).toBe(before);
  });

  it("does nothing more when the write fails", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = await idIn("acme-5");
    await user.click(id);
    expect(writeText).toHaveBeenCalled();
    expect(id).toHaveTextContent("#5");
    expect(window.location.href).toBe(before);
  });
});

describe("the All Tickets view", () => {
  beforeEach(() => localStorage.clear());

  async function ticketsWorld() {
    const w = await bySpecWorld();
    const s = scenario(w);
    // Core stamps a Ticket's creation with its clock, so each is made later than the last.
    w.advance(minutes(10));
    await s.ticket("acme", { title: "Old and done", spec: "acme-5", state: "Done" }); // acme-12
    w.advance(minutes(10));
    await s.human(`${s.projectPath("acme")}/submit_prototype`, { spec: "acme-5", text: "Sketch the menu" }); // acme-13
    w.advance(minutes(10));
    await s.ticket("beta", { title: "Beta newest", spec: "beta-1" }); // beta-3
    return w;
  }
  const allTickets = () => userEvent.click(within(nav()).getByRole("button", { name: /^All Tickets/ }));

  it("sits after Requirements, counting unfinished Tickets and following the Project filter", async () => {
    renderApp(await ticketsWorld());
    await within(nav()).findByRole("button", { name: /^Requirements/ });
    const names = within(nav()).getAllByRole("button").map((b) => b.textContent);
    const at = names.findIndex((n) => /^Requirements/.test(n ?? ""));
    expect(names[at + 1]).toMatch(/^All Tickets/);
    // acme: 4 Stages? seeded Tickets 1 Stage Done, the rest unfinished; the Done one and Done Stage excluded.
    const total = Number(within(nav()).getByRole("button", { name: /^All Tickets/ }).textContent!.replace(/\D/g, ""));
    await userEvent.click(within(nav()).getByRole("button", { name: /^Beta/ }));
    const beta = Number(within(nav()).getByRole("button", { name: /^All Tickets/ }).textContent!.replace(/\D/g, ""));
    expect(beta).toBe(2);
    expect(total).toBeGreaterThan(beta);
  });

  it("lists every kind of Ticket inline, newest first by creation across Projects, with chip, label and Spec line", async () => {
    renderApp(await ticketsWorld(), "/?view=inbox&finished=1");
    await within(nav()).findByRole("button", { name: /^All Tickets/ });
    await allTickets();
    expect(params().get("list")).toBe("tickets");
    expect(rowIds()[0]).toBe("beta-3");
    expect(rowIds()[1]).toBe("acme-13");
    expect(rowIds()[2]).toBe("acme-12"); // the oldest made after the rest
    expect(rowIds()).toContain("acme-3"); // a Stage Ticket, inline
    expect(within(list()).queryByText(/run by a Factory/)).toBeNull();

    const proto = within(list()).getByRole("link", { name: /acme-13/ });
    expect(proto).toHaveTextContent("Prototype");
    expect(proto).toHaveTextContent("Ready");
    expect(proto).toHaveTextContent("Spec");
    expect(proto).toHaveTextContent("acme-5");
    expect(proto).toHaveTextContent("Archive finished Projects");
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toHaveTextContent("spec");
    expect(within(list()).getByRole("link", { name: /acme-6/ })).not.toHaveTextContent(/Prototype|grilling/);
    expect(within(within(list()).getByRole("link", { name: /acme-7/ })).getByLabelText("Needs you")).toBeInTheDocument();
  });

  it("follows the Project filter and opens a Ticket on the right", async () => {
    renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&project=beta");
    await waitFor(() => expect(rowIds()).toEqual(["beta-3", "beta-2"]));
    await userEvent.click(within(list()).getByRole("link", { name: /Beta thing/ }));
    expect(params().get("ticket")).toBe("beta-2");
    expect(params().get("list")).toBe("tickets");
    expect(detail()).toHaveTextContent("Beta thing");
  });

  it("keeps the grouping toggle, which persists", async () => {
    renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
    await within(list()).findByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    expect(localStorage.length).toBe(1);
  });

  it("groups By Spec in the order of each Spec's newest Ticket, Stage Tickets first, with State chips", async () => {
    renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&group=spec&project=acme&finished=1");
    const rows = await within(list()).findAllByRole("region");
    // acme-5's newest Ticket (acme-13) is newer than any of the Requirement's.
    expect(rows[0]).toHaveAccessibleName("Archive finished Projects");
    expect(within(rows[0]).queryByText("Prototype: Sketch the menu")).toBeNull(); // collapsed
    await userEvent.click(within(rows[0]).getByRole("button", { name: "Archive finished Projects" }));
    const plainIds = within(rows[0]).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
    expect(plainIds.indexOf("acme-12")).toBeLessThan(plainIds.indexOf("acme-13")); // oldest first
    expect(within(rows[0]).getByText("Prototype: Sketch the menu").closest("a")).toHaveTextContent("Ready");

    const req = within(list()).getByRole("region", { name: "Export a Spec as Markdown" });
    await userEvent.click(within(req).getByRole("button", { name: "Export a Spec as Markdown" }));
    const ids = within(req).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
    expect(ids.slice(1)).toEqual(["acme-2", "acme-3", "acme-4", "acme-11"]); // Stages first, in Stage order
  });

  it("switching the grouping in a State list switches it here", async () => {
    renderApp(await ticketsWorld(), "/?state=Ready");
    await within(list()).findByRole("button", { name: "Spec" });
    await bySpec();
    await allTickets();
    expect(params().get("group")).toBe("spec");
    expect(await within(list()).findAllByRole("region")).not.toHaveLength(0);
  });

  it("hides Done and Cancelled until Show finished, kept in the URL and per browser", async () => {
    renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
    await within(list()).findByRole("button", { name: "Show finished" });
    await waitFor(() => expect(rowIds()).not.toContain("acme-12"));
    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    expect(params().get("finished")).toBe("1");
    await waitFor(() => expect(rowIds()).toContain("acme-12"));
    expect(localStorage.getItem("code-central.inbox.finished")).toBe("1");

    window.history.replaceState(null, "", "/?view=inbox&list=tickets");
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    await waitFor(() => expect(within(list()).getByRole("button", { name: "Show finished" })).toHaveAttribute("aria-pressed", "true"));

    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    expect(params().has("finished")).toBe(false);
    await waitFor(() => expect(rowIds()).not.toContain("acme-12"));
  });

  describe("search by ID", () => {
    const box = () => within(list()).getByRole("searchbox", { name: "Ticket ID" });
    const search = (text: string) => userEvent.type(box(), text);

    it("matches substrings and prefixes of the full ID, case-insensitively, across Projects, finished included", async () => {
      renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
      await within(list()).findByRole("searchbox", { name: "Ticket ID" });
      await search("ACME-1");
      await waitFor(() => expect(rowIds()).toContain("acme-12")); // Done, Show finished off
      expect(rowIds().every((id) => id?.includes("acme-1"))).toBe(true);
      await userEvent.clear(box());
      await search("3");
      await waitFor(() => expect(rowIds()).toContain("beta-3"));
      expect(rowIds()).toContain("acme-3");
      expect(window.location.search).not.toContain("3");
    });

    it("never matches titles and says No Tickets match.", async () => {
      renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
      await within(list()).findByRole("searchbox", { name: "Ticket ID" });
      await search("Sketch");
      expect(await within(list()).findByText("No Tickets match.")).toBeInTheDocument();
    });

    it("By Spec shows a Spec with its matching Tickets, or all its Tickets when its own ID matches, opened", async () => {
      renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&group=spec");
      await within(list()).findByRole("searchbox", { name: "Ticket ID" });
      await search("acme-12");
      const row = await within(list()).findByRole("region", { name: "Archive finished Projects" });
      expect(within(row).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean).slice(1)).toEqual(["acme-12"]); // after the Spec's own link
      await userEvent.clear(box());
      await search("acme-5");
      const whole = await within(list()).findByRole("region", { name: "Archive finished Projects" });
      const ids = within(whole).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
      expect(ids).toEqual(expect.arrayContaining(["acme-12", "acme-13"]));
    });
  });

  it("leaves out fully finished Specs By Spec", async () => {
    const w = await ticketsWorld();
    const s = scenario(w);
    await s.ticket("beta", { title: "Beta shipped", spec: await s.spec("beta", { title: "Shipped plan" }), state: "Done" });
    renderApp(w, "/?view=inbox&list=tickets&group=spec");
    await within(list()).findAllByRole("region");
    expect(within(list()).queryByRole("region", { name: "Shipped plan" })).toBeNull();
    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    expect(await within(list()).findByRole("region", { name: "Shipped plan" })).toBeInTheDocument();
  });

  it("says No Tickets yet. with Show finished on and nothing there", async () => {
    const { w } = await startWorld("acme");
    renderApp(w, "/?view=inbox&list=tickets&finished=1");
    expect(await within(list()).findByText("No Tickets yet.")).toBeInTheDocument();
  });

  it("says so when nothing is there", async () => {
    const { w } = await startWorld("acme");
    renderApp(w, "/?view=inbox&list=tickets");
    expect(await within(list()).findByText("No unfinished Tickets.")).toBeInTheDocument();
  });
});

describe("the CI badge on Ticket rows", () => {
  // ciWorld is a Spec (acme-1) with an Accepted Ticket whose CI is running
  // (acme-2), one with none (acme-3) and a Done one that passed (acme-4).
  async function ciWorld() {
    const { w, s } = await startWorld("acme");
    const spec = await s.spec("acme", { title: "Theming" });
    const accept = async (title: string, commit: string) => {
      const id = await s.ticket("acme", { title, spec });
      await s.accepted("acme", id, { branch: title.toLowerCase(), commit });
      return id;
    };
    const palette = await accept("Palette", "aaa1111");
    await s.pushed("acme", palette, "aaa1111");
    await s.ci("acme", palette, "running", "aaa1111", 1);
    await accept("Fonts", "bbb2222");
    const icons = await accept("Icons", "ccc3333");
    await s.pushed("acme", icons, "ccc3333");
    await s.ci("acme", icons, "passed", "ccc3333", 2);
    await s.human(`${s.ticketPath("acme", icons)}/land`, { commit: "ccc3333" });
    return w;
  }
  const row = async (title: string) => (await within(list()).findByText(title)).closest("a")!;

  it("shows on a State list's rows, and not on a row with no CI result", async () => {
    renderApp(await ciWorld(), "/?view=inbox&state=Accepted");
    expect(await row("Palette")).toHaveTextContent("CI running");
    expect(await row("Fonts")).not.toHaveTextContent("CI");
  });

  it("shows on the rows under a Spec row", async () => {
    renderApp(await ciWorld(), "/?view=inbox&state=Accepted&group=spec");
    await userEvent.click(await within(list()).findByRole("button", { name: "Theming" }));
    expect(await row("Palette")).toHaveTextContent("CI running");
  });

  it("shows in All Tickets, but not on a Done Ticket with Show finished on", async () => {
    renderApp(await ciWorld(), "/?view=inbox&list=tickets&finished=1");
    expect(await row("Palette")).toHaveTextContent("CI running");
    expect(await row("Icons")).not.toHaveTextContent("CI");
  });
});
