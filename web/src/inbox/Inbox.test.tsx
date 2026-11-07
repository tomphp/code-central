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
