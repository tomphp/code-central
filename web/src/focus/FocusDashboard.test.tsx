// Activity Dashboard tests: the real App against FakeApi, at ?view=focus,
// with the Focus Mode Option at its default, Activity Dashboard. They
// check what the human sees: where the dashboard shows, its counts and
// its In Progress list.
import { screen, within, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { ApiError } from "../api/types";
import { change, focus, renderApp } from "./focusTesting";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Login form", state: "Ready" }); // acme-1
  fake.addTicket("acme", { title: "Sign-up page", state: "In Progress" }); // acme-2
  fake.addTicket("beta", { title: "Beta export", state: "Ready" }); // beta-1
  fake.addTicket("acme", { title: "Old work", state: "Done" }); // acme-3
  return fake;
}

const dashboard = () => within(focus()).findByRole("region", { name: "Activity Dashboard" });
// block is state's block, named by its State; count is the number on it.
const block = (d: HTMLElement, state: string) => within(d).getByRole("group", { name: state });
const count = (d: HTMLElement, state: string) => within(block(d, state)).getByTestId("count").textContent;

describe("where the Activity Dashboard shows", () => {
  it("shows where All clear would by default", async () => {
    renderApp(world());
    const d = await dashboard();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  it("shows All clear, never the dashboard, until the Options are read", async () => {
    const fake = world();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const options = fake.options.bind(fake);
    vi.spyOn(fake, "options").mockImplementation(async () => {
      await gate;
      return options();
    });
    renderApp(fake);
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    release();
    expect(await dashboard()).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("shows All clear, not the dashboard, when the Options can't be read", async () => {
    const fake = world();
    vi.spyOn(fake, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    renderApp(fake);
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
    expect(screen.queryByText("Code Central isn't reachable")).toBeNull();
  });

  it("switches live between the dashboard and All clear on options.changed, with no reload", async () => {
    const fake = world();
    renderApp(fake);
    await dashboard();

    await act(() => fake.setOptions({ focus_clear: "all_clear" }));
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    await act(() => fake.setOptions({ focus_clear: "activity_dashboard" }));
    expect(await dashboard()).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("gives way to a Focus Item the moment one appears, and comes back when it goes", async () => {
    const fake = world();
    renderApp(fake);
    await dashboard();

    change(fake, [{ reason: "failed", tickets: [{ id: "acme-2" }] }], "acme-2");
    expect(await within(focus()).findByRole("article", { name: "Focus Item" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();

    change(fake, [], "acme-2");
    expect(await dashboard()).toBeInTheDocument();
  });

  it("leaves the skipped screen as it is", async () => {
    const fake = world();
    fake.updateTicket("acme-2", { state: "Failed" });
    fake.setNeedsMe([{ reason: "failed", tickets: [{ id: "acme-2" }] }]);
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

    expect(await within(focus()).findByRole("region", { name: "Only skipped Focus Items left" })).toHaveTextContent("You skipped 1, go through them again?");
    expect(within(focus()).queryByRole("region", { name: "Activity Dashboard" })).toBeNull();
  });
});

describe("the counts", () => {
  it("cover only Focus Mode's Project when scoped, and every Project when not", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=focus&project=beta");
    let d = await dashboard();
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
    unmount();

    renderApp(fake);
    d = await dashboard();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
  });

  it("leave out Archived Projects", async () => {
    const fake = world();
    fake.addProject("old", "Old", { archived: true });
    fake.addTicket("old", { title: "Left behind", state: "Ready" });
    fake.addTicket("old", { title: "Still going", state: "In Progress" });
    renderApp(fake);
    const d = await dashboard();
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("1");
    expect(within(d).queryByText("Still going")).toBeNull();
  });

  it("include Stage Tickets and blocked Ready Tickets, and follow a change live", async () => {
    const fake = world();
    const spec = fake.addSpec("acme", { title: "Dark mode" });
    fake.addTicket("acme", { title: "Grilling: Dark mode", state: "In Progress", spec: { id: spec.id, title: spec.title }, labels: ["stage:grilling"] });
    fake.addTicket("acme", { title: "Waits on login", state: "Ready", blocked: true });
    renderApp(fake);
    const d = await dashboard();
    expect(count(d, "Ready")).toBe("3");
    expect(count(d, "In Progress")).toBe("2");

    act(() => void fake.updateTicket("acme-1", { state: "In Progress" }));
    await within(d).findByText("Login form");
    expect(count(d, "Ready")).toBe("2");
    expect(count(d, "In Progress")).toBe("3");
  });
});

// busy is world with more work In Progress, out of order: acme-12 with a
// Session posting events, beta-2, and acme-11 a Stage Ticket of the
// Dark mode Requirement (acme-10).
function busy() {
  const fake = world();
  fake.addTicket("beta", { title: "Beta import", state: "In Progress" }); // beta-2
  const spec = fake.addSpec("acme", { id: "acme-10", title: "Dark mode" });
  fake.addTicket("acme", { title: "Spec: Dark mode", state: "In Progress", spec: { id: spec.id, title: spec.title }, labels: ["stage:spec"] }); // acme-11
  const claims = [{ number: 1, session_id: "s-busy", started: fake.now.toISOString(), last_activity: fake.now.toISOString() }];
  fake.addTicket("acme", { title: "Theme toggle", state: "In Progress", claims }); // acme-12
  fake.postSessionEvents("s-busy", [{ time: new Date(fake.now.getTime() - 2 * 60_000).toISOString(), kind: "tool_call", tool: "Read", text: "Read: web/src/App.tsx" }]);
  return fake;
}

const inProgress = (d: HTMLElement) => within(d).getByRole("list", { name: "In Progress" });
const entries = (d: HTMLElement) => within(inProgress(d)).getAllByRole("listitem");

describe("the In Progress list", () => {
  it("lists the In Progress Tickets by Project, then Ticket number", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    expect(entries(d).map((e) => within(e).getByText(/^(acme|beta)-\d+$/).textContent)).toEqual(["acme-2", "acme-11", "acme-12", "beta-2"]);
  });

  it("shows each entry's ID, title, Project and latest event line", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    const [quiet, , theme, beta] = entries(d);
    expect(quiet).toHaveTextContent(/^acme-2Sign-up pageAcme$/);
    expect(theme).toHaveTextContent(/^acme-12Theme toggleAcmeRead: web\/src\/App.tsx · 2m ago$/);
    expect(beta).toHaveTextContent(/^beta-2Beta importBeta$/);
  });

  it("shows a Stage Ticket's Requirement and Stage", async () => {
    const fake = busy();
    renderApp(fake);
    const stage = entries(await dashboard())[1];
    expect(within(stage).getByText("Requirement: Dark mode · Spec")).toBeInTheDocument();
  });

  it("leaves out the Project when scoped to one", async () => {
    const fake = busy();
    renderApp(fake, "/?view=focus&project=acme");
    const d = await dashboard();
    expect(entries(d)[0]).toHaveTextContent(/^acme-2Sign-up page$/);
    expect(entries(d)).toHaveLength(3);
  });

  it("follows the live feed: a new latest event, and a Ticket leaving In Progress", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    act(() => fake.postSessionEvents("s-busy", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    expect(await within(d).findByText("Writing the test first. · just now")).toBeInTheDocument();

    act(() => void fake.updateTicket("beta-2", { state: "In Review" }));
    await waitFor(() => expect(count(d, "In Progress")).toBe("3"));
    expect(within(d).queryByText("Beta import")).toBeNull();
  });

  it("says Nothing in progress when it's empty, with the blocks still showing", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus&project=beta");
    const d = await dashboard();
    expect(within(d).getByText("Nothing in progress")).toBeInTheDocument();
    expect(within(d).queryByRole("listitem")).toBeNull();
    expect(count(d, "Ready")).toBe("1");
    expect(count(d, "In Progress")).toBe("0");
  });

  it("opens an entry's Ticket in the Inbox, keeping the Project filter", async () => {
    const fake = busy();
    renderApp(fake, "/?view=focus&project=acme");
    await userEvent.click(within(entries(await dashboard())[0]).getByRole("link"));
    expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-2");
    expect(await screen.findByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket in its Requirement", async () => {
    const fake = busy();
    renderApp(fake);
    await userEvent.click(within(entries(await dashboard())[1]).getByRole("link"));
    expect(window.location.search).toBe("?view=inbox&requirement=acme-10&ticket=acme-11");
  });
});

// pulsing is whether el shows it is pulsing.
const pulsing = (el: HTMLElement) => el.getAttribute("data-pulsing") === "true";
const entry = (d: HTMLElement, id: string) => within(inProgress(d)).getByText(id).closest("li") as HTMLElement;

describe("pulses", () => {
  it("marks neither a block nor an entry on first show", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
    expect(entries(d).filter(pulsing)).toEqual([]);
  });

  it("marks a block when its count changes, and not the other", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => void fake.updateTicket("beta-1", { state: "Done" }));
    await waitFor(() => expect(count(d, "Ready")).toBe("1"));
    expect(pulsing(block(d, "Ready"))).toBe(true);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks only the entry whose Ticket a notice names, and no block when a refetch leaves the counts the same", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => fake.postSessionEvents("s-busy", [{ time: fake.now.toISOString(), kind: "text", text: "Writing the test first." }]));
    await within(d).findByText("Writing the test first. · just now");
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "acme-12")]);
    expect(pulsing(block(d, "Ready"))).toBe(false);
    expect(pulsing(block(d, "In Progress"))).toBe(false);
  });

  it("marks an entry on a notice that changes nothing it shows", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => fake.emit({ type: "ticket.changed", project: "beta", id: "beta-2" }));
    await waitFor(() => expect(pulsing(entry(d, "beta-2"))).toBe(true));
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "beta-2")]);
  });

  it("marks an entry that joins the list", async () => {
    const fake = busy();
    renderApp(fake);
    const d = await dashboard();

    act(() => void fake.updateTicket("acme-1", { state: "In Progress" }));
    await within(d).findByText("Login form");
    expect(entries(d).filter(pulsing)).toEqual([entry(d, "acme-1")]);
    expect(pulsing(block(d, "In Progress"))).toBe(true);
  });

  it("marks the first entry to join an empty list", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus&project=beta");
    const d = await dashboard();

    act(() => void fake.updateTicket("beta-1", { state: "In Progress" }));
    await within(d).findByText("Beta export");
    expect(pulsing(entry(d, "beta-1"))).toBe(true);
  });
});
