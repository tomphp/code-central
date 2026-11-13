// Board view tests: the real App against FakeApi, at ?view=board. They
// check what the human sees and does: the Board link and its count, the
// columns and Columns ▾ with which columns are hidden and where that comes
// from, the cards, a card's Ticket popup and closing it, the shared
// grouping, + New ▾ and Back to the Inbox.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { ApiError, STATES } from "../api/types";

const at = (minutes: number) => new Date(Date.UTC(2026, 10, 7, 9, minutes)).toISOString();

// world seeds acme with a Spec (acme-1) and Tickets in several States,
// created a minute apart, and beta with one Ticket.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  const login = fake.addSpec("acme", { title: "Login" }); // acme-1
  const spec = { id: login.id, title: login.title };
  fake.addTicket("acme", { title: "Login form", state: "Ready", spec, created: at(1), updated: at(1) }); // acme-2
  fake.addTicket("acme", { title: "Sign-up page", state: "In Progress", spec, created: at(2), updated: at(2) }); // acme-3
  fake.addTicket("acme", { title: "Old work", state: "Done", spec, created: at(3), updated: at(3) }); // acme-4
  fake.addTicket("acme", { title: "Dropped", state: "Cancelled", spec, created: at(4), updated: at(4) }); // acme-5
  fake.addTicket("acme", { title: "Password reset", state: "Ready", spec, created: at(5), updated: at(5), blocked: true, labels: ["prototype"] }); // acme-6
  fake.addTicket("beta", { title: "Beta export", state: "In Progress", created: at(6), updated: at(6) }); // beta-1
  return fake;
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const boardLink = () => within(nav()).getByRole("link", { name: /^Board/ });
const board = () => screen.getByRole("main", { name: "Board" });
const heads = () => within(board()).getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
const column = (state: string) => within(board()).getByRole("region", { name: state });
const cards = (state: string) => within(column(state)).queryAllByRole("link");
const titles = (state: string) => cards(state).map((c) => c.querySelector(".board-card-title")?.textContent);
const card = (title: string) => within(board()).getByRole("link", { name: new RegExp(title) });
const columnsButton = () => within(board()).getByRole("button", { name: /^Columns/ });
const loaded = () => within(board()).findByRole("link", { name: /Login form/ });

beforeEach(() => localStorage.clear());

describe("the Board link", () => {
  it("sits in the View section after All Tickets and its States, before Admin, on the Inbox's Project", async () => {
    renderApp(world(), "/?view=inbox&project=acme&list=tickets");
    const link = boardLink();
    const allTickets = within(nav()).getByRole("button", { name: /^All Tickets/ });
    const lastState = within(nav()).getByRole("button", { name: /^Cancelled/ });
    const admin = within(nav()).getByRole("heading", { name: "Admin" });
    expect(allTickets.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(lastState.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(admin) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link).toHaveAttribute("href", "?view=board&project=acme&back=view%3Dinbox%26project%3Dacme%26list%3Dtickets");
  });

  it("counts the Tickets in the columns the Board opens with, which by default is the All Tickets count", async () => {
    renderApp(world(), "/?view=inbox&project=acme");
    await waitFor(() => expect(boardLink()).toHaveTextContent("Board 3"));
    expect(within(nav()).getByRole("button", { name: /^All Tickets/ })).toHaveTextContent("All Tickets 3");
  });

  it("counts every Project's Tickets on All Projects, and carries the Inbox's grouping", async () => {
    renderApp(world(), "/?view=inbox&group=spec");
    await waitFor(() => expect(boardLink()).toHaveTextContent("Board 4"));
    expect(boardLink()).toHaveAttribute("href", "?view=board&group=spec&back=view%3Dinbox%26group%3Dspec");
  });

  it("follows the columns the browser last chose", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await userEvent.click(columnsButton());
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Done" }));
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Ready" }));
    unmount();

    renderApp(fake, "/?view=inbox&project=acme");
    await waitFor(() => expect(boardLink()).toHaveTextContent("Board 2"));
  });

  it("opens the Board on the Inbox's Project, and Back to the Inbox returns to the Inbox as it was", async () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Ready");
    await userEvent.click(boardLink());
    expect(within(board()).getByRole("combobox", { name: "Projects" })).toHaveValue("acme");
    expect(await loaded()).toBeInTheDocument();

    await userEvent.click(within(board()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Ready");
    expect(nav()).toBeInTheDocument();
  });
});

describe("the Board's columns", () => {
  it("shows one column per State in workflow order, Done and Cancelled hidden, each with its count", async () => {
    renderApp(world(), "/?view=board&project=acme");
    await loaded();
    expect(heads()).toEqual(["Draft 0", "Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0"]);
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
  });

  it("reads No Tickets in an empty shown column", async () => {
    renderApp(world(), "/?view=board&project=acme");
    await loaded();
    expect(column("Draft")).toHaveTextContent("No Tickets");
    expect(cards("Draft")).toEqual([]);
    expect(column("Ready")).not.toHaveTextContent("No Tickets");
  });

  it("shows and hides columns with Columns ▾, in the address, and shows · n/10 only while any is hidden", async () => {
    renderApp(world(), "/?view=board&project=acme");
    await loaded();
    await userEvent.click(columnsButton());
    expect(within(board()).getAllByRole("checkbox")).toEqual(STATES.map((s) => within(board()).getByRole("checkbox", { name: s })));
    expect(within(board()).getByRole("checkbox", { name: "Done" })).not.toBeChecked();
    expect(within(board()).getByRole("checkbox", { name: "Ready" })).toBeChecked();

    await userEvent.click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(heads()).toContain("Done 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("Cancelled");
    expect(columnsButton()).toHaveTextContent("Columns · 9/10");

    await userEvent.click(within(board()).getByRole("checkbox", { name: "Cancelled" }));
    expect(heads()).toHaveLength(10);
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("");
    expect(columnsButton()).toHaveTextContent(/^Columns ▾$/);

    await userEvent.click(within(board()).getByRole("checkbox", { name: "In Progress" }));
    expect(heads()).not.toContain("In Progress 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("In Progress");
  });

  it("keeps the browser's pick for a fresh Board with no hide in the address", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await userEvent.click(columnsButton());
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Done" }));
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Draft" }));
    unmount();

    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    expect(heads()).toEqual(["Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0", "Done 1"]);
  });

  it("lets the address's hidden columns win over the browser's pick", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await userEvent.click(columnsButton());
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Done" }));
    unmount();

    renderApp(fake, "/?view=board&project=acme&hide=Draft,Ready,Accepted");
    await waitFor(() => expect(heads()).toEqual(["In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Done 1", "Cancelled 1"]));
  });

  it("shows every column for an empty hide=", async () => {
    renderApp(world(), "/?view=board&project=acme&hide=");
    await loaded();
    expect(heads()).toHaveLength(10);
    expect(heads()).toContain("Done 1");
    expect(heads()).toContain("Cancelled 1");
  });

  it("follows the Projects picker, keeping the address", async () => {
    renderApp(world(), "/?view=board");
    await loaded();
    expect(titles("In Progress")).toEqual(["Beta export", "Sign-up page"]);
    const picker = within(board()).getByRole("combobox", { name: "Projects" });
    await userEvent.selectOptions(picker, "beta");
    expect(window.location.search).toBe("?view=board&project=beta");
    expect(titles("In Progress")).toEqual(["Beta export"]);
    expect(cards("Ready")).toEqual([]);
  });
});

describe("the Board's cards", () => {
  it("show the ID, kind, Blocked, the needs-you dot and the Spec's kind and name", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-6" }] }]);
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    const reset = card("Password reset");
    expect(within(reset).getByRole("button", { name: "acme-6" })).toHaveTextContent("#6");
    expect(reset).toHaveTextContent("Prototype");
    expect(reset).toHaveTextContent("⛔ Blocked");
    expect(within(reset).getByRole("img", { name: "Needs you" })).toBeInTheDocument();
    expect(reset).toHaveTextContent(/Spec\s*Login$/);

    const form = card("Login form");
    expect(within(form).getByRole("button", { name: "acme-2" })).toBeInTheDocument();
    expect(form).not.toHaveTextContent("Prototype");
    expect(form).not.toHaveTextContent("Blocked");
    expect(within(form).queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("show the CI badge of a not-Done Ticket, none for no CI result or a Done Ticket, and follow the live feed", async () => {
    const fake = world();
    fake.updateTicket("acme-2", { ci_result: "running" });
    fake.updateTicket("acme-3", { ci_result: "failed" });
    fake.updateTicket("acme-4", { ci_result: "passed" });
    renderApp(fake, "/?view=board&project=acme&hide=Cancelled");
    await loaded();
    expect(card("Login form")).toHaveTextContent("CI running");
    expect(card("Sign-up page")).toHaveTextContent("CI failed");
    expect(card("Password reset")).not.toHaveTextContent("CI");
    expect(card("Old work")).not.toHaveTextContent("CI");
    await act(async () => fake.updateTicket("acme-2", { ci_result: "passed" }));
    await waitFor(() => expect(card("Login form")).toHaveTextContent("CI passed"));
  });

  it("show full IDs on All Projects, and no Spec line for a Ticket on no Spec", async () => {
    renderApp(world(), "/?view=board");
    await loaded();
    expect(within(card("Login form")).getByRole("button", { name: "acme-2" })).toHaveTextContent("acme-2");
    expect(card("Beta export")).toHaveTextContent(/^beta-1\s*Beta export$/);
  });

  it("are newest first by creation", async () => {
    renderApp(world(), "/?view=board&project=acme");
    await loaded();
    expect(titles("Ready")).toEqual(["Password reset", "Login form"]);
  });

  it("move column when a Ticket changes State, and pulse then but not on first show", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    expect(within(board()).getAllByRole("link").filter((c) => c.hasAttribute("data-pulsing"))).toEqual([]);

    fake.now = new Date(at(30));
    await act(async () => fake.updateTicket("acme-2", { state: "In Progress" }));
    await waitFor(() => expect(titles("In Progress")).toEqual(["Sign-up page", "Login form"]));
    expect(titles("Ready")).toEqual(["Password reset"]);
    expect(card("Login form")).toHaveAttribute("data-pulsing", "true");
    expect(card("Sign-up page")).not.toHaveAttribute("data-pulsing");
  });

  it("pulse when a Ticket changes in place", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();

    fake.now = new Date(at(30));
    await act(async () => fake.updateTicket("acme-3", { title: "Sign-up page, redone" }));
    await waitFor(() => expect(card("Sign-up page, redone")).toHaveAttribute("data-pulsing", "true"));
    expect(card("Login form")).not.toHaveAttribute("data-pulsing");
  });

  it("don't pulse when a hidden column is shown", async () => {
    renderApp(world(), "/?view=board&project=acme");
    await loaded();
    await userEvent.click(columnsButton());
    await userEvent.click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(card("Old work")).not.toHaveAttribute("data-pulsing");
  });

  it("link to the Board's own address with their popup, not into the Inbox", async () => {
    renderApp(world(), "/?view=board&project=acme&group=spec&hide=");
    await loaded();
    expect(card("Login form")).toHaveAttribute("href", "?view=board&project=acme&group=spec&hide=&popup=acme-2");
    expect(card("Old work")).toHaveAttribute("href", "?view=board&project=acme&group=spec&hide=&popup=acme-4");
  });
});

describe("the Board's Ticket popup", () => {
  const popup = () => screen.getByRole("dialog", { name: "Ticket popup" });
  const noPopup = () => expect(screen.queryByRole("dialog")).toBeNull();
  // popupHeading waits for the Ticket popup to show its Ticket's title.
  const popupHeading = async (title: string) =>
    within(await screen.findByRole("dialog", { name: "Ticket popup" })).findByRole("heading", { name: new RegExp(title) });
  const openCard = async (title: string) => {
    await userEvent.click(within(card(title)).getByText(title));
    return popupHeading(title);
  };

  it("opens from a card that doesn't need the human, with the Ticket's pane and actions, in the address", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Rough idea", state: "Draft", created: at(7), updated: at(7) }); // acme-7
    renderApp(fake, "/?view=board&project=acme&hide=Done");
    await loaded();
    expect(await openCard("Rough idea")).toBeInTheDocument();
    expect(window.location.search).toBe("?view=board&project=acme&hide=Done&popup=acme-7");
    expect(within(popup()).getByRole("button", { name: "Edit" })).toBeInTheDocument();
    expect(within(popup()).getByRole("article", { name: "Ticket details" })).toBeInTheDocument();
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(within(popup()).queryByRole("link", { name: /Open in Inbox/ })).toBeNull();
    expect(within(popup()).queryByRole("button", { name: /Open in Inbox|Skip/ })).toBeNull();
    expect(popup()).not.toHaveTextContent(/\d+ left/);
    // The Board stays behind it, as it was.
    expect(heads()).toContain("Cancelled 1");
  });

  it("shows Loading… until the read arrives, and the read's error if it fails", async () => {
    const fake = world();
    let fail: (e: Error) => void = () => {};
    fake.getTicket = () => new Promise((_, reject) => (fail = reject));
    renderApp(fake, "/?view=board&project=acme&popup=acme-2");
    await loaded();
    expect(popup()).toHaveTextContent("Loading…");
    await act(async () => fail(new ApiError(404, "not_found", "Ticket acme-2 not found")));
    expect(popup()).toHaveTextContent("Ticket acme-2 not found");
  });

  it("offers no edits for a Ticket of an Archived Project", async () => {
    const fake = world();
    fake.addProject("old", "Old", { archived: true });
    fake.addTicket("old", { title: "Legacy", state: "Draft" }); // old-1
    renderApp(fake, "/?view=board&project=old&popup=old-1");
    expect(await popupHeading("Legacy")).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("follows the live feed", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await openCard("Login form");
    await act(async () => fake.updateTicket("acme-2", { title: "Login form, redone" }));
    expect(await within(popup()).findByRole("heading", { name: /Login form, redone/ })).toBeInTheDocument();
  });

  it("stays a Ticket popup when its Ticket starts needing the human", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await openCard("Login form");
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-2" }] }]);
    await act(async () => fake.updateTicket("acme-2", { state: "Needs Answer" }));
    await waitFor(() => expect(within(board()).getByRole("img", { name: "Needs you" })).toBeInTheDocument());
    expect(within(popup()).getByRole("heading", { name: /Login form/ })).toBeInTheDocument();
  });

  it("switches to its Spec's popup address from the Spec link", async () => {
    renderApp(world(), "/?view=board&project=acme&group=spec");
    await loaded();
    await openCard("Login form");
    await userEvent.click(within(popup()).getByRole("button", { name: "acme-1 Login" }));
    expect(window.location.search).toBe("?view=board&project=acme&group=spec&popup=acme-1");
  });

  it.each([
    ["×", () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => userEvent.keyboard("{Escape}")],
    ["the backdrop", () => userEvent.click(popup().parentElement!)],
  ])("closes on %s, pushing the Board's address without popup and keeping its grouping and hidden columns", async (_, close) => {
    renderApp(world(), "/?view=board&project=acme&group=spec&hide=Draft");
    await loaded();
    const length = window.history.length;
    await openCard("Login form");
    await close();
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme&group=spec&hide=Draft");
    expect(window.history.length).toBe(length + 2);
    expect(heads()[0]).toBe("Ready 2");
    expect(card("Login form")).toHaveFocus();
  });

  it("closes on the browser's Back, and Forward opens it again", async () => {
    renderApp(world(), "/?view=board&project=acme&hide=Draft");
    await loaded();
    await openCard("Login form");
    act(() => window.history.back());
    await waitFor(noPopup);
    expect(window.location.search).toBe("?view=board&project=acme&hide=Draft");
    expect(card("Login form")).toHaveFocus();

    act(() => window.history.forward());
    expect(await popupHeading("Login form")).toBeInTheDocument();
  });

  it("reopens when the address with popup= is loaded", async () => {
    renderApp(world(), "/?view=board&project=acme&popup=acme-3");
    expect(await popupHeading("Sign-up page")).toBeInTheDocument();
  });

  it("opens on any Project's Ticket, whatever the picker is on", async () => {
    renderApp(world(), "/?view=board&project=acme&popup=beta-1");
    expect(await popupHeading("Beta export")).toBeInTheDocument();
  });

  it("opens nothing for an unknown ID, and leaves the address alone", async () => {
    renderApp(world(), "/?view=board&project=acme&popup=acme-99");
    await loaded();
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme&popup=acme-99");
  });

  it("shows nothing while the Inbox data loads", async () => {
    const fake = world();
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    const needsMe = fake.needsMe.bind(fake);
    fake.needsMe = async (...args) => {
      await gate;
      return needsMe(...args);
    };
    renderApp(fake, "/?view=board&project=acme&popup=acme-2");
    expect(within(board()).getByText("Loading…")).toBeInTheDocument();
    noPopup();
    await act(async () => release());
    expect(await screen.findByRole("dialog", { name: "Ticket popup" })).toBeInTheDocument();
  });
});

describe("the Board's grouping", () => {
  it("is By Ticket by default, and shares its pick with the State lists", async () => {
    const fake = world();
    const { unmount } = renderApp(fake, "/?view=board&project=acme");
    await loaded();
    const toggle = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(toggle).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(toggle).getByRole("button", { name: "Spec" }));
    expect(within(toggle).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    expect(new URLSearchParams(window.location.search).get("group")).toBe("spec");
    unmount();

    const inbox = renderApp(fake, "/?view=inbox&project=acme&state=Ready");
    const list = await within(screen.getByRole("main", { name: "Tickets" })).findByRole("group", { name: "Group the list" });
    expect(within(list).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(list).getByRole("button", { name: "Ticket" }));
    inbox.unmount();

    renderApp(fake, "/?view=board&project=acme");
    const again = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(again).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("the Board's By Spec lanes", () => {
  // lanes seeds a second Spec, newer than Login, with a Ready and an In Progress Ticket,
  // and a third whose only Ticket is Done.
  function lanesWorld() {
    const fake = world();
    const billing = fake.addSpec("acme", { title: "Billing" }); // acme-7
    const spec = { id: billing.id, title: billing.title };
    fake.addTicket("acme", { title: "Invoice list", state: "Ready", spec, created: at(7), updated: at(7) }); // acme-8
    fake.addTicket("acme", { title: "Refunds", state: "In Progress", spec, created: at(8), updated: at(8) }); // acme-9
    const archive = fake.addSpec("acme", { title: "Archive" }); // acme-10
    fake.addTicket("acme", { title: "Cold storage", state: "Done", spec: { id: archive.id, title: archive.title }, created: at(9), updated: at(9) }); // acme-11
    return fake;
  }
  const lanes = () => within(board()).getAllByRole("group", { name: /^Lane: / }).map((l) => l.getAttribute("aria-label"));
  const lane = (title: string) => within(board()).getByRole("group", { name: `Lane: ${title}` });
  const bySpec = (url = "/?view=board&project=acme&group=spec") => renderApp(lanesWorld(), url);

  it("shows one lane per Spec with a Ticket in a shown column, newest Spec first, and no lanes By Ticket", async () => {
    bySpec();
    await loaded();
    expect(lanes()).toEqual(["Lane: Billing", "Lane: Login"]);
    await userEvent.click(within(within(board()).getByRole("group", { name: "Group the board" })).getByRole("button", { name: "Ticket" }));
    expect(within(board()).queryAllByRole("group", { name: /^Lane: / })).toEqual([]);
    expect(cards("Ready")).toHaveLength(3);
  });

  it("adds a lane when its Spec's Done Tickets are shown with Columns ▾", async () => {
    bySpec("/?view=board&project=acme&group=spec&hide=");
    await loaded();
    expect(lanes()).toEqual(["Lane: Archive", "Lane: Billing", "Lane: Login"]);
  });

  it("heads each lane with its Lane header, which opens the Spec", async () => {
    bySpec();
    await loaded();
    const row = within(lane("Login")).getByRole("region", { name: "Login" });
    // One line: the count of the Tickets the lane shows, and no State chips.
    expect(within(row).getByText("3 shown")).toBeInTheDocument();
    expect(row.querySelector(".state-chips, .strip")).toBeNull();
    await userEvent.click(within(row).getByRole("link", { name: /Login/ }));
    const q = new URLSearchParams(window.location.search);
    expect(q.get("view")).toBe("inbox");
    expect(q.get("project")).toBe("acme");
    expect(q.get("requirement")).toBe("acme-1");
  });

  it("puts each Ticket's card in its lane's column, without the Spec line", async () => {
    bySpec();
    await loaded();
    const ready = within(lane("Login")).getByRole("region", { name: "Ready" });
    expect(within(ready).getAllByRole("link").map((c) => c.querySelector(".board-card-title")?.textContent)).toEqual(["Password reset", "Login form"]);
    const billingReady = within(lane("Billing")).getByRole("region", { name: "Ready" });
    expect(within(billingReady).getByRole("link", { name: /Invoice list/ })).not.toHaveTextContent("Billing");
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ }).querySelector(".board-card-spec")).toBeNull();
  });

  it("counts a column's Tickets across all lanes in its head", async () => {
    bySpec();
    await loaded();
    expect(heads()).toContain("Ready 3");
    expect(heads()).toContain("In Progress 2");
    expect(heads()).toContain("Draft 0");
  });

  it("collapses a lane to its header, keeping its per-column counts and the head totals", async () => {
    bySpec();
    await loaded();
    const toggle = within(lane("Login")).getByRole("button", { name: "Login" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await userEvent.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Login")).queryAllByRole("link", { name: /Login form|Sign-up page|Password reset/ })).toEqual([]);
    expect(within(within(lane("Login")).getByRole("region", { name: "Ready" })).getByText("2")).toBeInTheDocument();
    expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByText("1")).toBeInTheDocument();
    expect(within(lane("Billing")).getByRole("link", { name: /Invoice list/ })).toBeInTheDocument();
    expect(heads()).toContain("Ready 3");
    await userEvent.click(toggle);
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ })).toBeInTheDocument();
  });

  it("keeps folded lanes across a card's popup opening and closing", async () => {
    bySpec();
    await loaded();
    await userEvent.click(within(lane("Billing")).getByRole("button", { name: "Billing" }));
    await userEvent.click(within(lane("Login")).getByText("Login form"));
    await screen.findByRole("dialog", { name: "Ticket popup" });
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(lane("Billing")).getByRole("button", { name: "Billing" })).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Billing")).queryByRole("link", { name: /Invoice list/ })).toBeNull();
  });

  it("follows a Ticket moving column inside its lane", async () => {
    const fake = lanesWorld();
    renderApp(fake, "/?view=board&project=acme&group=spec");
    await loaded();
    act(() => void fake.updateTicket("acme-2", { state: "In Progress" }));
    await waitFor(() => expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByRole("link", { name: /Login form/ })).toBeInTheDocument());
    expect(heads()).toContain("In Progress 3");
  });
});

describe("+ New ▾ on the Board", () => {
  it("submits a Requirement and opens its Spec in the Requirements list", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await userEvent.click(within(board()).getByRole("button", { name: "More to create" }));
    await userEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const q = new URLSearchParams(window.location.search);
    expect(q.get("view")).toBe("inbox");
    expect(q.get("project")).toBe("acme");
    expect(q.get("list")).toBe("requirements");
    expect(q.get("requirement")).toBe("acme-7");
  });

  it("submits a Small Change and opens its Ticket in the Inbox", async () => {
    const fake = world();
    renderApp(fake, "/?view=board&project=acme");
    await loaded();
    await userEvent.click(within(board()).getByRole("button", { name: "More to create" }));
    await userEvent.click(within(screen.getByRole("menu")).getByRole("menuitem", { name: "Small Change" }));
    const modal = screen.getByRole("dialog", { name: "New Small Change" });
    await userEvent.type(within(modal).getByRole("textbox", { name: "Small Change" }), "Rename the button");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit small change" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const q = new URLSearchParams(window.location.search);
    expect(q.get("view")).toBe("inbox");
    expect(q.get("project")).toBe("acme");
    expect(q.get("ticket")).toBe("acme-8");
  });
});
