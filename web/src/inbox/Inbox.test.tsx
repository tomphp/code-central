// Inbox layout tests: the real App and panels against FakeApi. They check
// layout only; what the service computes (group order, reasons) is the
// fake's seeded data, rendered as received.
import { render, screen, within, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

const minutesAgo = (fake: FakeApi, m: number) => new Date(fake.now.getTime() - m * 60_000).toISOString();

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer" }); // acme-1
  fake.addTicket("acme", { title: "Fix login", state: "Failed" }); // acme-2
  fake.addTicket("acme", { title: "Rough idea", state: "Draft" }); // acme-3
  fake.addTicket("acme", { title: "Acme ready", state: "Ready" }); // acme-4
  fake.addTicket("beta", { title: "Beta review", state: "In Review" }); // beta-1
  fake.addTicket("beta", { title: "Beta ready", state: "Ready" }); // beta-2
  fake.addTicket("beta", { title: "Beta sketch", state: "Draft" }); // beta-3
  // Deliberately not the service's usual order: the UI must render as received.
  fake.setNeedsMe([
    { reason: "failed", tickets: [{ id: "acme-2", since: minutesAgo(fake, 90), failure: { headline: "Tests fail on CI", reports: 2 } }] },
    { reason: "needs_answer", tickets: [{ id: "acme-1", since: minutesAgo(fake, 5), round: { number: 1, open: 1, questions: 2 } }] },
    { reason: "in_review", tickets: [{ id: "beta-1", since: minutesAgo(fake, 3 * 60), summary: "Added the login page" }] },
  ]);
  return fake;
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
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
    renderApp(world());
    await waitFor(() => expect(groupNames()).toEqual(["failed", "needs_answer", "in_review"]));

    const row = within(list()).getByRole("link", { name: /acme-2/ });
    expect(row).toHaveTextContent("Fix login");
    expect(row).toHaveTextContent("1h");
    expect(row).toHaveTextContent("Tests fail on CI · failed 2×");
    expect(within(list()).getByRole("link", { name: /acme-1/ })).toHaveTextContent("1 of 2 open · Round 1");
  });

  it("keeps Drafts below in a collapsed group", async () => {
    renderApp(world());
    const drafts = await within(list()).findByText(/Drafts \(2\)/);
    const group = drafts.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    expect(within(group).getByRole("link", { name: /acme-3/, hidden: true })).not.toBeVisible();
    // Drafts come after every Needs Me group.
    const sections = [...list().querySelectorAll("section, details")];
    expect(sections.at(-1)).toBe(group);
  });

  it("shows a State as a flat list with a dot on rows that need the human", async () => {
    const fake = world();
    fake.updateTicket("acme-4", { state: "Failed" }); // a second Failed Ticket, not in Needs Me
    renderApp(fake);
    await userEvent.click(await within(nav()).findByRole("button", { name: /^Failed/ }));

    await waitFor(() => expect(rowIds()).toEqual(["acme-2", "acme-4"]));
    expect(groupNames()).toEqual([]);
    expect(within(list()).getByRole("link", { name: /acme-2/ })).toContainElement(screen.getByLabelText("Needs you"));
    expect(within(within(list()).getByRole("link", { name: /acme-4/ })).queryByLabelText("Needs you")).toBeNull();
    expect(params().get("state")).toBe("Failed");
  });

  it("lists Projects with their Needs Me counts and a View list with counts", async () => {
    renderApp(world());
    await waitFor(() => expect(within(nav()).getByRole("button", { name: "Acme 2" })).toBeInTheDocument());
    expect(within(nav()).getByRole("button", { name: "Beta 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "All Projects 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Needs Me 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 2" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Done 0" })).toBeInTheDocument();
  });

  it("combines the Project and View filters", async () => {
    renderApp(world());
    await userEvent.click(await within(nav()).findByRole("button", { name: /^Beta/ }));
    await waitFor(() => expect(rowIds()).toEqual(["beta-1", "beta-3"])); // Needs Me, then Drafts
    expect(within(nav()).getByRole("button", { name: "Needs Me 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 1" })).toBeInTheDocument();

    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    await waitFor(() => expect(rowIds()).toEqual(["beta-2"]));
    expect(params().get("project")).toBe("beta");
    expect(params().get("state")).toBe("Ready");

    await userEvent.click(within(nav()).getByRole("button", { name: /^All Projects/ }));
    await waitFor(() => expect(rowIds()).toEqual(["acme-4", "beta-2"]));
  });

  it("puts the view and the selected Ticket in the URL, and opens them from it", async () => {
    const fake = world();
    const { unmount } = renderApp(fake);
    await userEvent.click(await within(list()).findByRole("link", { name: /acme-1/ }));
    expect(params().get("view")).toBe("inbox");
    expect(params().get("ticket")).toBe("acme-1");
    expect(await within(detail()).findByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
    unmount();

    renderApp(fake, "/?view=inbox&state=Ready&ticket=beta-2");
    expect(await within(detail()).findByRole("heading", { name: "Beta ready" })).toBeInTheDocument();
    await waitFor(() => expect(rowIds()).toEqual(["acme-4", "beta-2"]));
    expect(within(list()).getByRole("link", { name: /beta-2/ })).toHaveAttribute("aria-current", "true");
  });

  it("shows the selected Ticket with its action panel slot first", async () => {
    renderApp(world(), "/?view=inbox&ticket=acme-2");
    const heading = await within(detail()).findByRole("heading", { name: "Fix login" });
    const slot = within(detail()).getByRole("region", { name: "Action panel" });
    const body = within(detail()).getByRole("article", { name: "Ticket details" });
    expect(slot.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(heading.closest("aside")).toBe(detail());
    expect(within(detail()).getByText("Failed", { selector: ".state" })).toBeInTheDocument();
  });

  it("refetches after each change notice", async () => {
    const fake = world();
    renderApp(fake);
    await waitFor(() => expect(rowIds()).toContain("acme-1"));

    const t = fake.addTicket("beta", { title: "Another sketch" }); // a Draft; sends ticket.changed
    await waitFor(() => expect(rowIds()).toContain(t.id));
    expect(within(list()).getByText(/Drafts \(3\)/)).toBeInTheDocument();
  });

  it("refetches Needs Me after a notice and everything after a reconnect", async () => {
    const fake = world();
    renderApp(fake, "/?view=inbox&ticket=beta-2");
    await waitFor(() => expect(rowIds()).toContain("acme-1"));
    await within(detail()).findByRole("heading", { name: "Beta ready" });

    fake.setNeedsMe([]);
    act(() => fake.emit({ type: "ticket.changed", project: "acme", id: "acme-1" }));
    await waitFor(() => expect(groupNames()).toEqual([]));
    expect(screen.getByText("Nothing needs you.")).toBeInTheDocument();

    // A missed notice: the feed drops, the Ticket changes, the feed returns.
    fake.updateTicket("beta-2", { title: "Beta renamed" });
    fake.calls.length = 0;
    act(() => fake.reconnect());
    expect(await within(detail()).findByRole("heading", { name: "Beta renamed" })).toBeInTheDocument();
    expect(fake.calls).toEqual(expect.arrayContaining(["listProjects", "needsMe", "findTickets", "getTicket"]));
  });

  it("shows the Needs Me count in the tab title and favicon", async () => {
    const fake = world();
    renderApp(fake);
    await waitFor(() => expect(document.title).toBe("(3) Code Central"));
    const icon = () => document.querySelector<HTMLLinkElement>("link[rel=icon]")!;
    expect(decodeURIComponent(icon().href)).toContain(">3<");

    fake.setNeedsMe([]);
    act(() => fake.emit({ type: "ticket.changed", project: "acme", id: "acme-1" }));
    await waitFor(() => expect(document.title).toBe("Code Central"));
    expect(decodeURIComponent(icon().href)).not.toMatch(/>\d+</);
  });
});

// requirements is a Project with three Requirements and a plain Ticket:
// acme-1 at Spec, which needs an answer; acme-5 in Build, with two
// implementation Tickets needing the human; acme-11 waiting for a Factory
// to claim Grilling.
async function requirements() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  await fake.submitRequirement("acme", "Export a Spec as Markdown"); // acme-1, stages acme-2..4
  fake.updateTicket("acme-2", { state: "Done" });
  fake.updateTicket("acme-3", { state: "Needs Answer" });
  const title = "Archive finished Projects";
  const building = await fake.submitRequirement("acme", title); // acme-5, stages acme-6..8
  for (const t of building.stages) fake.updateTicket(t.id, { state: "Done" });
  const spec = { id: building.spec.id, title };
  fake.addTicket("acme", { title: "Archive in the Project menu", state: "In Review", spec }); // acme-9
  fake.addTicket("acme", { title: "CLI: archive-project", state: "Failed", spec }); // acme-10
  await fake.submitRequirement("acme", "Render Markdown"); // acme-11, stages acme-12..14
  fake.addTicket("acme", { title: "Plain work", state: "Failed" }); // acme-15
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-3", round: { number: 1, open: 1, questions: 1 } }] },
    {
      reason: "failed",
      tickets: [
        { id: "acme-10", failure: { headline: "Tests fail", reports: 1 } },
        { id: "acme-15", failure: { headline: "Lint", reports: 1 } },
      ],
    },
    { reason: "in_review", tickets: [{ id: "acme-9", summary: "Menu item added" }] },
  ]);
  return fake;
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
    const fake = await requirements();
    const done = await fake.submitRequirement("acme", "Finished work"); // acme-16
    for (const t of done.stages) fake.updateTicket(t.id, { state: "Done" });
    renderApp(fake);
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
    expect(within(failed).getByRole("link", { name: /acme-15/ })).not.toHaveTextContent("↳");
    const review = within(list()).getByRole("region", { name: "In Review" });
    expect(within(review).getByRole("link", { name: /acme-9/ })).toHaveTextContent("↳ Archive finished Projects");
  });

  it("counts a Requirement row once in Needs Me, in place of its Stage Ticket", async () => {
    renderApp(await requirements());
    // acme-1's row, acme-9, acme-10 and acme-15.
    await waitFor(() => expect(within(nav()).getByRole("button", { name: "Needs Me 4" })).toBeInTheDocument());
  });

  it("counts Requirement rows, not Stage Tickets, in the tab title and All Projects too", async () => {
    const fake = await requirements();
    // A second Stage Ticket of acme-1 needing the human is still one row.
    fake.updateTicket("acme-2", { state: "Failed" });
    fake.setNeedsMe([
      { reason: "needs_answer", tickets: [{ id: "acme-3", round: { number: 1, open: 1, questions: 1 } }] },
      {
        reason: "failed",
        tickets: [
          { id: "acme-2", failure: { headline: "Lost", reports: 1 } },
          { id: "acme-10", failure: { headline: "Tests fail", reports: 1 } },
          { id: "acme-15", failure: { headline: "Lint", reports: 1 } },
        ],
      },
      { reason: "in_review", tickets: [{ id: "acme-9", summary: "Menu item added" }] },
    ]);
    renderApp(fake);
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
    const fake = await requirements();
    fake.addTicket("acme", { title: "Shipped", state: "Done" }); // acme-16
    renderApp(fake, "/?view=inbox&state=Done");
    // Besides acme-16, acme-2 and the three Stages of acme-5 are Done.
    await waitFor(() => expect(within(list()).getByText("Stage Tickets (4): run by a Factory")).toBeInTheDocument());
    expect(within(nav()).getByRole("button", { name: "Done 1" })).toBeInTheDocument();
    expect(rowIds()[0]).toBe("acme-16");
    expect(rowIds()).toHaveLength(5);
    expect(within(list()).getByRole("link", { name: /acme-16/ })).toBeVisible();
  });
});

// inProgress is two In Progress Tickets with live Claims, and a Needs
// Answer one whose live Claim has a latest event too.
function inProgress() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const claim = (session_id: string) => [{ number: 1, session_id, started: minutesAgo(fake, 30), last_activity: minutesAgo(fake, 1) }];
  fake.addTicket("acme", { title: "Busy work", state: "In Progress", claims: claim("s-busy") }); // acme-1
  fake.addTicket("acme", { title: "Quiet work", state: "In Progress", claims: claim("s-quiet") }); // acme-2
  fake.addTicket("acme", { title: "Asking work", state: "Needs Answer", claims: claim("s-asking") }); // acme-3
  fake.postSessionEvents("s-busy", [{ time: minutesAgo(fake, 2), kind: "tool_call", tool: "Read", text: "Read: core/core.go" }]);
  fake.postSessionEvents("s-asking", [{ time: minutesAgo(fake, 2), kind: "factory", text: "acme-3 posted a Round" }]);
  return fake;
}

describe("the latest Session Event in the In Progress list", () => {
  it("shows each row's latest event faintly under its title, and nothing on a row with none", async () => {
    renderApp(inProgress(), "/?view=inbox&state=In%20Progress");
    const busy = await within(list()).findByRole("link", { name: /acme-1/ });
    const line = within(busy).getByText("Read: core/core.go · 2m ago");
    expect(line).toHaveClass("row-detail", "latest-event");
    const quiet = within(list()).getByRole("link", { name: /acme-2/ });
    expect(quiet.textContent).toBe("acme-2Quiet work");
  });

  it("updates on the live feed without a reload", async () => {
    const fake = inProgress();
    renderApp(fake, "/?view=inbox&state=In%20Progress");
    await within(list()).findByText("Read: core/core.go · 2m ago");
    act(() => fake.postSessionEvents("s-quiet", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    const quiet = within(list()).getByRole("link", { name: /acme-2/ });
    expect(await within(quiet).findByText("Writing the test first. · just now")).toBeInTheDocument();
  });

  it("leaves it off rows in other States", async () => {
    renderApp(inProgress(), "/?view=inbox&state=Needs%20Answer");
    const asking = await within(list()).findByRole("link", { name: /acme-3/ });
    expect(asking).not.toHaveTextContent("posted a Round");
  });
});

// wayfinding is a Map acme-1 whose Charting Ticket acme-2 needs an answer
// and whose Prototype acme-3 is In Review, and a Prototype acme-5 on the
// plain Spec acme-4, also In Review.
function wayfinding() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const map = fake.addSpec("acme", { title: "Find the way", labels: ["wayfinder:map"] }); // acme-1
  const onMap = { id: map.id, title: map.title };
  fake.addTicket("acme", { title: "Charting: Find the way", state: "Needs Answer", spec: onMap, labels: ["wayfinder:chart"] }); // acme-2
  fake.addTicket("acme", { title: "Prototype: the graph", state: "In Review", spec: onMap, labels: ["prototype"] }); // acme-3
  const plain = fake.addSpec("acme", { title: "Login" }); // acme-4
  fake.addTicket("acme", { title: "Prototype: the form", state: "In Review", spec: { id: plain.id, title: plain.title }, labels: ["prototype"] }); // acme-5
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-2", round: { number: 1, open: 1, questions: 1 } }] },
    { reason: "in_review", tickets: [{ id: "acme-3" }, { id: "acme-5" }] },
  ]);
  return fake;
}

describe("Map Tickets and Prototypes in Needs Me", () => {
  it("lists each as its own row with its Map's or Spec's name, and a Prototype badge on a Prototype", async () => {
    renderApp(wayfinding());
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

  it("shows + New, its ▾ and Focus at the top of the sidebar", async () => {
    renderApp(world());
    const newButton = await within(nav()).findByRole("button", { name: "New requirement" });
    expect(newButton).toHaveTextContent("+ New");
    expect(menuButton()).toHaveAttribute("aria-haspopup", "menu");
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(within(nav()).getByRole("button", { name: "Focus" })).toHaveTextContent("◎ Focus");
    expect(within(nav()).queryByRole("button", { name: "New Map" })).toBeNull();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens the New requirement dialog from + New", async () => {
    renderApp(world());
    await userEvent.click(await within(nav()).findByRole("button", { name: "New requirement" }));
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("opens a menu of Requirement and Map from ▾, each with its hint", async () => {
    renderApp(world());
    await userEvent.click(await findMenuButton());
    expect(menuButton()).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("menu", { name: "More to create" })).toBeVisible();
    const items = within(screen.getByRole("menu")).getAllByRole("menuitem");
    expect(items.map((i) => i.textContent)).toEqual([expect.stringContaining("Requirement"), expect.stringContaining("Map")]);
    expect(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Requirement" })).toHaveAccessibleDescription(
      "Grilled into a Spec and Tickets",
    );
    expect(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Map" })).toHaveAccessibleDescription(
      "Charts the decisions to get somewhere",
    );
  });

  it.each([
    ["Requirement", "New requirement", "Submit requirement", "submitRequirement"],
    ["Map", "New Map", "Submit map", "submitMap"],
  ])("opens %s's dialog from the menu, which stays visible once the menu closes, and submits to the new Spec", async (item, dialog, submit, call) => {
    const fake = world();
    renderApp(fake, "/?project=acme");
    await userEvent.click(await findMenuButton());
    await userEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: item }));

    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    const modal = screen.getByRole("dialog", { name: dialog });
    expect(modal).toBeVisible();
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await userEvent.click(within(modal).getByRole("button", { name: submit }));

    expect(fake.calls).toContain(call);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(params().get("list")).toBe("requirements");
    expect(params().get("requirement")).toBe("acme-5");
  });

  it("closes the menu on Esc and on a click outside it", async () => {
    renderApp(world());
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
    renderApp(world());
    await userEvent.click(await findMenuButton());
    await userEvent.click(within(nav()).getByRole("button", { name: "New requirement" }));
    expect(screen.queryByRole("menu")).toBeNull();
    expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("dialog", { name: "New requirement" })).toBeVisible();
  });

  it("toggles the menu shut from ▾", async () => {
    renderApp(world());
    await userEvent.click(await findMenuButton());
    await userEvent.click(menuButton());
    expect(screen.queryByRole("menu")).toBeNull();
  });

  describe("by keyboard", () => {
    const item = (name: string) => within(screen.getByRole("menu")).getByRole("menuitem", { name });

    it("opens with ↓ on the first item, and moves with the arrows, wrapping at either end", async () => {
      renderApp(world());
      (await findMenuButton()).focus();
      await userEvent.keyboard("{ArrowDown}");
      expect(menuButton()).toHaveAttribute("aria-expanded", "true");
      expect(item("Requirement")).toHaveFocus();

      await userEvent.keyboard("{ArrowDown}");
      expect(item("Map")).toHaveFocus();
      await userEvent.keyboard("{ArrowDown}");
      expect(item("Requirement")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Map")).toHaveFocus();
      await userEvent.keyboard("{ArrowUp}");
      expect(item("Requirement")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("opens with %s on the first item", async (key) => {
      renderApp(world());
      (await findMenuButton()).focus();
      await userEvent.keyboard(key);
      expect(item("Requirement")).toHaveFocus();
    });

    it.each([["{Enter}"], [" "]])("picks the focused item with %s: the menu closes and its dialog opens", async (key) => {
      renderApp(world());
      (await findMenuButton()).focus();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}");
      await userEvent.keyboard(key);
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(screen.getByRole("dialog", { name: "New Map" })).toBeVisible();
    });

    it("closes on Esc and returns focus to ▾", async () => {
      renderApp(world());
      (await findMenuButton()).focus();
      await userEvent.keyboard("{ArrowDown}{ArrowDown}{Escape}");
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
      expect(menuButton()).toHaveFocus();
    });

    it("closes when Tab leaves the menu", async () => {
      renderApp(world());
      (await findMenuButton()).focus();
      await userEvent.keyboard("{ArrowDown}");
      await userEvent.tab();
      expect(screen.queryByRole("menu")).toBeNull();
      expect(menuButton()).toHaveAttribute("aria-expanded", "false");
    });
  });

  it("enters Focus Mode from Focus", async () => {
    renderApp(world());
    await userEvent.click(await within(nav()).findByRole("button", { name: "Focus" }));
    expect(params().get("view")).toBe("focus");
  });
});

describe("the Admin section", () => {
  const follows = (a: Element, b: Element) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_PRECEDING) !== 0;

  it("keeps the title row to the heading, with no Usage or Options links", () => {
    renderApp(world());
    const title = within(nav()).getByRole("heading", { level: 1, name: "Code Central" });
    expect(within(title.parentElement!).queryByRole("link")).toBeNull();
  });

  it("follows View's last State filter and precedes the Archived Projects", async () => {
    const fake = world();
    fake.addProject("old", "Old", { archived: true });
    renderApp(fake);
    const admin = within(nav()).getByRole("heading", { level: 2, name: "Admin" });
    const lastState = within(nav()).getByRole("button", { name: /^Cancelled/ });
    const archived = await within(nav()).findByText(/^Archived/);
    expect(follows(admin, lastState)).toBe(true);
    expect(follows(archived, admin)).toBe(true);

    const usage = within(nav()).getByRole("link", { name: "Usage" });
    const options = within(nav()).getByRole("link", { name: "Options" });
    expect(follows(usage, admin)).toBe(true);
    expect(follows(options, usage)).toBe(true);
    expect(follows(archived, options)).toBe(true);
  });

  it("links Usage and Options with real addresses that return to the Inbox", () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).toHaveAttribute("href", "?view=usage&back=view%3Dinbox%26project%3Dacme%26state%3DReady");
    expect(within(nav()).getByRole("link", { name: "Options" })).toHaveAttribute("href", "?view=options&from=inbox&project=acme&state=Ready");
    expect(within(nav()).getByRole("link", { name: "Usage" })).not.toHaveAttribute("aria-current");
  });

  it("opens Usage and Options", async () => {
    renderApp(world());
    await userEvent.click(within(nav()).getByRole("link", { name: "Usage" }));
    expect(screen.getByRole("main", { name: "Usage" })).toBeInTheDocument();

    act(() => window.history.back());
    await userEvent.click(within(await screen.findByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Options" }));
    expect(screen.getByRole("main", { name: "Options" })).toBeInTheDocument();
  });
});
