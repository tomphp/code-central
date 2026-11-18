// Inbox layout tests for Needs Me and the State views: the triage inbox, Requirements,
// Stage Tickets, the In Progress list, and Map Tickets and Prototypes.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { scenario, settle } from "../worldTesting";
import { minutes, startWorld, requirement, doneStages, world, renderApp, cancelAll, list, nav, detail, groupNames, rowIds, params, requirements, groupHeads } from "./Inbox.testutil";

describe("the triage inbox", () => {
  it("renders Needs Me groups in the order the service returns, each row with its wait and detail", async () => {
    await renderApp(await world());
    expect(groupNames()).toEqual(["needs_answer", "failed", "in_review"]);

    const row = within(list()).getByRole("link", { name: /acme-3/ });
    expect(row).toHaveTextContent("Fix login");
    expect(row).toHaveTextContent("1h");
    expect(row).toHaveTextContent("Tests fail on CI · failed 2×");
    expect(within(list()).getByRole("link", { name: /acme-2/ })).toHaveTextContent("1 of 2 open · Round 1");
  });

  it("keeps Drafts below in a collapsed group", async () => {
    await renderApp(await world());
    const drafts = within(list()).getByText(/Drafts \(2\)/);
    const group = drafts.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    expect(within(group).getByRole("link", { name: /acme-4/, hidden: true })).not.toBeVisible();
    // Drafts come after every Needs Me group.
    const sections = [...list().querySelectorAll("section, details")];
    expect(sections.at(-1)).toBe(group);
  });

  it("shows a State as a flat list with a dot on rows that need the human", async () => {
    // Core puts every Failed Ticket in Needs Me, so the row with no dot is a Ready one.
    const w = await world();
    await renderApp(w);
    await userEvent.click(within(nav()).getByRole("button", { name: /^Failed/ }));
    await settle(w);

    expect(rowIds()).toEqual(["acme-3"]);
    expect(groupNames()).toEqual([]);
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toContainElement(screen.getByLabelText("Needs you"));
    expect(params().get("state")).toBe("Failed");

    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    await settle(w);
    expect(rowIds()).toEqual(["acme-5", "beta-3"]);
    expect(within(within(list()).getByRole("link", { name: /acme-5/ })).queryByLabelText("Needs you")).toBeNull();
  });

  it("lists Projects with their Needs Me counts and a View list with counts", async () => {
    await renderApp(await world());
    expect(within(nav()).getByRole("button", { name: "Acme 2" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Beta 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "All Projects 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Needs Me 3" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 2" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Done 0" })).toBeInTheDocument();
  });

  it("combines the Project and View filters", async () => {
    const w = await world();
    await renderApp(w);
    await userEvent.click(within(nav()).getByRole("button", { name: /^Beta/ }));
    await settle(w);
    expect(rowIds()).toEqual(["beta-2", "beta-4"]); // Needs Me, then Drafts
    expect(within(nav()).getByRole("button", { name: "Needs Me 1" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "Ready 1" })).toBeInTheDocument();

    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    await settle(w);
    expect(rowIds()).toEqual(["beta-3"]);
    expect(params().get("project")).toBe("beta");
    expect(params().get("state")).toBe("Ready");

    await userEvent.click(within(nav()).getByRole("button", { name: /^All Projects/ }));
    await settle(w);
    expect(rowIds()).toEqual(["acme-5", "beta-3"]);
  });

  it("puts the view and the selected Ticket in the URL, and opens them from it", async () => {
    const w = await world();
    const { unmount } = await renderApp(w);
    await userEvent.click(within(list()).getByRole("link", { name: /acme-2/ }));
    await settle(w);
    expect(params().get("view")).toBe("inbox");
    expect(params().get("ticket")).toBe("acme-2");
    expect(within(detail()).getByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
    unmount();

    await renderApp(w, "/?view=inbox&state=Ready&ticket=beta-3");
    expect(within(detail()).getByRole("heading", { name: "Beta ready" })).toBeInTheDocument();
    expect(rowIds()).toEqual(["acme-5", "beta-3"]);
    expect(within(list()).getByRole("link", { name: /beta-3/ })).toHaveAttribute("aria-current", "true");
  });

  it("shows the selected Ticket with its action panel slot first", async () => {
    await renderApp(await world(), "/?view=inbox&ticket=acme-3");
    const heading = within(detail()).getByRole("heading", { name: "Fix login" });
    const slot = within(detail()).getByRole("region", { name: "Action panel" });
    const body = within(detail()).getByRole("article", { name: "Ticket details" });
    expect(slot.compareDocumentPosition(body) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(heading.closest("aside")).toBe(detail());
    expect(within(detail()).getByText("Failed", { selector: ".state-chip" })).toBeInTheDocument();
  });

  it("refetches after each change notice", async () => {
    const w = await world();
    await renderApp(w);
    expect(rowIds()).toContain("acme-2");

    const id = await scenario(w).ticket("beta", { title: "Another sketch", state: "Draft" }); // sends ticket.changed
    expect(rowIds()).toContain(id);
    expect(within(list()).getByText(/Drafts \(3\)/)).toBeInTheDocument();
  });

  it("refetches Needs Me after a notice and everything after a reconnect", async () => {
    const w = await world();
    await renderApp(w, "/?view=inbox&ticket=beta-3");
    expect(rowIds()).toContain("acme-2");
    within(detail()).getByRole("heading", { name: "Beta ready" });

    await cancelAll(w, "acme", ["acme-2", "acme-3"]);
    await cancelAll(w, "beta", ["beta-2"]);
    expect(groupNames()).toEqual([]);
    expect(screen.getByText("Nothing needs you.")).toBeInTheDocument();

    // A missed notice: the feed drops, the Ticket changes, the feed returns.
    w.calls.length = 0;
    w.dropStreams();
    const s = scenario(w);
    await s.human(`${s.ticketPath("beta", "beta-3")}/edit_ticket`, { title: "Beta renamed" });
    expect(within(detail()).getByRole("heading", { name: "Beta renamed" })).toBeInTheDocument();
    expect(w.calls).toEqual(expect.arrayContaining(["listProjects", "needsMe", "findTickets", "getTicket"]));
  });

  it("shows the Needs Me count in the tab title and favicon", async () => {
    const w = await world();
    await renderApp(w);
    expect(document.title).toBe("(3) Code Central");
    const icon = () => document.querySelector<HTMLLinkElement>("link[rel=icon]")!;
    expect(decodeURIComponent(icon().href)).toContain(">3<");

    await cancelAll(w, "acme", ["acme-2", "acme-3"]);
    await cancelAll(w, "beta", ["beta-2"]);
    expect(document.title).toBe("Code Central");
    expect(decodeURIComponent(icon().href)).not.toMatch(/>\d+</);
  });
});

describe("Requirements in Needs Me", () => {
  it("shows a Requirement whose Stage needs the human as one row in a Requirements group at the top, never the Stage Ticket", async () => {
    await renderApp(await requirements());
    expect(groupHeads()).toEqual(["Requirements 1", "Failed 2", "In Review 1"]);
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
    await renderApp(w);
    const summary = within(list()).getByText("Requirements in flight (1): not waiting on you");
    const group = summary.closest("details")!;
    expect(group).not.toHaveAttribute("open");
    const row = within(group).getByRole("link", { name: /acme-11/, hidden: true });
    expect(row).not.toBeVisible();
    expect(row).toHaveTextContent("Grilling: waiting for a Factory");
    // Neither the Requirement in Build, whose Tickets show as rows, nor the finished one.
    expect([...group.querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id)).toEqual(["acme-11"]);
  });

  it("opens a Requirement row on the Stage that needs the human", async () => {
    const w = await requirements();
    await renderApp(w);
    await userEvent.click(within(list()).getByRole("link", { name: /Export a Spec as Markdown/ }));
    await settle(w);
    expect(params().get("requirement")).toBe("acme-1");
    expect(within(detail()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
    expect(within(detail()).getByRole("heading", { name: "Spec: Export a Spec as Markdown" })).toBeInTheDocument();
  });

  it("keeps implementation Tickets as ordinary rows naming their Requirement, several at once", async () => {
    await renderApp(await requirements());
    const failed = within(list()).getByRole("region", { name: "Failed" });
    expect(within(failed).getByRole("link", { name: /acme-10/ })).toHaveTextContent("Archive finished Projects");
    expect(within(failed).getByRole("link", { name: /acme-16/ }).querySelector(".req-chip")).toBeNull();
    const review = within(list()).getByRole("region", { name: "In Review" });
    expect(within(review).getByRole("link", { name: /acme-9/ })).toHaveTextContent("Archive finished Projects");
  });

  it("counts a Requirement row once in Needs Me, in place of its Stage Ticket", async () => {
    await renderApp(await requirements());
    // acme-1's row, acme-9, acme-10 and acme-16.
    expect(within(nav()).getByRole("button", { name: "Needs Me 4" })).toBeInTheDocument();
  });

  it("counts Requirement rows, not Stage Tickets, in the tab title and All Projects too", async () => {
    const w = await requirements();
    // Core runs a Requirement's Stages one at a time, so only one Stage Ticket
    // of acme-1 can need the human; it is one row.
    await renderApp(w);
    expect(within(nav()).getByRole("button", { name: "Needs Me 4" })).toBeInTheDocument();
    expect(within(nav()).getByRole("button", { name: "All Projects 4" })).toBeInTheDocument();
    expect(document.title).toBe("(4) Code Central");
  });
});

describe("Stage Tickets in State views", () => {
  it("folds them into a collapsed group and leaves them out of the State counts", async () => {
    await renderApp(await requirements());
    // Ready: acme-4 and acme-12 are Stage Tickets; nothing else is Ready.
    await userEvent.click(within(nav()).getByRole("button", { name: "Ready 0" }));
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
    await renderApp(w, "/?view=inbox&state=Done");
    // Besides acme-17, acme-2 and the three Stages of acme-5 are Done.
    expect(within(list()).getByText("Stage Tickets (4): run by a Factory")).toBeInTheDocument();
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
    await renderApp((await inProgress()).w, "/?view=inbox&state=In%20Progress");
    const busy = within(list()).getByRole("link", { name: /acme-2/ });
    const line = within(busy).getByText("Read: core/core.go · 2m ago");
    expect(line).toHaveClass("row-detail", "latest-event");
    const quiet = within(list()).getByRole("link", { name: /acme-3/ });
    expect(quiet.textContent).toBe("acme-3Quiet work");
  });

  it("updates on the live feed without a reload", async () => {
    const { w, s } = await inProgress();
    await renderApp(w, "/?view=inbox&state=In%20Progress");
    within(list()).getByText("Read: core/core.go · 2m ago");
    await s.postSessionEvents("s-quiet", [{ time: w.now().toISOString(), kind: "text", text: "Writing the test first." }]);
    const quiet = within(list()).getByRole("link", { name: /acme-3/ });
    expect(within(quiet).getByText("Writing the test first. · just now")).toBeInTheDocument();
  });

  it("leaves it off rows in other States", async () => {
    await renderApp((await inProgress()).w, "/?view=inbox&state=Needs%20Answer");
    const asking = within(list()).getByRole("link", { name: /acme-4/ });
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
    await renderApp(await wayfinding());
    expect(rowIds()).toEqual(["acme-2", "acme-3", "acme-5"]);
    expect(groupHeads()).toEqual(["Questions 1", "In Review 2"]);
    const row = (id: string) => within(list()).getByRole("link", { name: new RegExp(id) });
    expect(row("acme-2")).toHaveTextContent("Map: Find the way");
    expect(within(row("acme-2")).queryByText("Prototype")).toBeNull();
    expect(row("acme-3")).toHaveTextContent("Map: Find the way");
    expect(within(row("acme-3")).getByText("Prototype")).toHaveClass("label", "prototype-badge");
    expect(row("acme-5")).toHaveTextContent("Login");
    expect(row("acme-5")).not.toHaveTextContent("Map:");
    expect(within(row("acme-5")).getByText("Prototype")).toHaveClass("prototype-badge");
  });
});
