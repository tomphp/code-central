// Board view tests: the real App on a WASM Core world, at ?view=board. They
// check what the human sees and does: the Board link and its count, the
// columns and Columns ▾ with which columns are hidden and where that comes
// from, the cards, a card's Ticket popup or Focus popup and closing it, the
// Focus popup's Questions and closing by itself, a Lane header's Spec popup,
// the shared grouping, + New ▾ and Back to the Inbox.
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { Api } from "../api/api";
import { newWorld, nextTask, type World } from "../api/core/core";
import { WORK, type QuestionSeed, type Scenario } from "../api/scenario";
import { ApiError, STATES } from "../api/types";
import { failCI } from "../ciTesting";
import { scenario, settle } from "../worldTesting";

const MINUTE = 60_000;

// Fake is a world, its scenario builder and the IDs of what world made.
interface Fake {
  w: World;
  s: Scenario;
  ids: { spec: string; form: string; signup: string; old: string; dropped: string; reset: string; beta: string };
}

// world makes acme with a Spec (acme-1) and Tickets in several States,
// created a minute apart, and beta with one Ticket.
async function world(): Promise<Fake> {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const spec = await s.spec("acme", { title: "Login" }); // acme-1
  const next = () => w.advance(MINUTE);
  next();
  const form = await s.ticket("acme", { title: "Login form", state: "Ready", spec }); // acme-2
  next();
  const signup = await s.ticket("acme", { title: "Sign-up page", state: "In Progress", spec }); // acme-3
  next();
  const old = await s.ticket("acme", { title: "Old work", state: "Done", spec }); // acme-4
  next();
  const dropped = await s.ticket("acme", { title: "Dropped", state: "Cancelled", spec }); // acme-5
  next();
  const reset = await s.ticket("acme", { title: "Password reset", state: "Ready", spec, labels: ["prototype"] }); // acme-6
  await s.addBlocker("acme", reset, signup);
  next();
  const beta = await s.ticket("beta", { title: "Beta export", state: "In Progress" });
  next();
  return { w, s, ids: { spec, form, signup, old, dropped, reset, beta } };
}

// renderApp renders the App at url on f's world, with over's Api methods in
// place of the world's.
async function renderApp(f: Fake, url = "/", over: Partial<Api> = {}) {
  rendered = f.w;
  window.history.replaceState(null, "", url);
  const api = new Proxy(f.w.api, { get: (t, k: string) => (k in over ? over[k as keyof Api] : t[k as keyof Api]) });
  const r = render(<App api={api} now={f.w.now} />);
  await settle(f.w);
  return r;
}

// rendered is the world renderApp last rendered over; settled waits for it
// after a user-event interaction or a direct call on its Api.
let rendered: World;
const settled = () => settle(rendered);
// click is a user's click, settled.
const click = async (el: Element) => {
  await userEvent.click(el);
  await settled();
};
// travel is the browser's Back or Forward, settled: jsdom fires popstate a
// task after the call.
const travel = async (go: () => void) => {
  await act(async () => {
    go();
    await nextTask();
  });
  await settled();
};
// press is a key press, settled.
const press = async (keys: string) => {
  await userEvent.keyboard(keys);
  await settled();
};

// needing has Sign-up page (In Progress) asking a Round and Login form
// (Ready) In Review: both need the human, in Needs Me's order.
async function needing(f: Fake) {
  await f.s.askRound("acme", f.ids.signup);
  await f.s.inReview("acme", f.ids.form);
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
const loaded = () => within(board()).getByRole("link", { name: /Login form/ });
// noPopup checks no popup, nor any other dialog, is open.
const noPopup = () => expect(screen.queryByRole("dialog")).toBeNull();

beforeEach(() => localStorage.clear());

describe("the Board link", () => {
  it("sits in the View section right after Activity, before Needs Me, on the Inbox's Project", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme&list=tickets");
    const link = boardLink();
    const activity = within(nav()).getByRole("link", { name: "Activity" });
    const needsMe = within(nav()).getByRole("button", { name: /^Needs Me/ });
    expect(activity.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(needsMe) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link).toHaveAttribute("href", "?view=board&project=acme&back=view%3Dinbox%26project%3Dacme%26list%3Dtickets");
  });

  it("counts the Tickets in the columns the Board opens with, which by default is the All Tickets count", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 3");
    expect(within(nav()).getByRole("button", { name: /^All Tickets/ })).toHaveTextContent("All Tickets 3");
  });

  it("counts every Project's Tickets on All Projects, and carries the Inbox's grouping", async () => {
    await renderApp(await world(), "/?view=inbox&group=spec");
    expect(boardLink()).toHaveTextContent("Board 4");
    expect(boardLink()).toHaveAttribute("href", "?view=board&group=spec&back=view%3Dinbox%26group%3Dspec");
  });

  it("follows the columns the browser last chose", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    await click(within(board()).getByRole("checkbox", { name: "Ready" }));
    unmount();

    await renderApp(fake, "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 2");
  });

  it("opens the Board on the Inbox's Project, and Back to the Inbox returns to the Inbox as it was", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme&state=Ready");
    await click(boardLink());
    expect(within(board()).getByRole("combobox", { name: "Projects" })).toHaveValue("acme");
    expect(loaded()).toBeInTheDocument();

    await click(within(board()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Ready");
    expect(nav()).toBeInTheDocument();
  });
});

describe("the Board's columns", () => {
  it("shows one column per State in workflow order, Done and Cancelled hidden, each with its count", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(heads()).toEqual(["Draft 0", "Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0"]);
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
  });

  it("reads No Tickets in an empty shown column", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(column("Draft")).toHaveTextContent("No Tickets");
    expect(cards("Draft")).toEqual([]);
    expect(column("Ready")).not.toHaveTextContent("No Tickets");
  });

  it("shows and hides columns with Columns ▾, in the address, and shows · n/10 only while any is hidden", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    expect(within(board()).getAllByRole("checkbox")).toEqual(
      STATES.flatMap((s) => [within(board()).getByRole("checkbox", { name: s }), ...(s === "In Progress" ? [within(board()).getByRole("checkbox", { name: "Needs Me" })] : [])]),
    );
    expect(within(board()).getByRole("checkbox", { name: "Needs Me" })).not.toBeChecked();
    expect(within(board()).getByRole("checkbox", { name: "Done" })).not.toBeChecked();
    expect(within(board()).getByRole("checkbox", { name: "Ready" })).toBeChecked();

    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(heads()).toContain("Done 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("Cancelled");
    expect(columnsButton()).toHaveTextContent("Columns · 9/10");

    await click(within(board()).getByRole("checkbox", { name: "Cancelled" }));
    expect(heads()).toHaveLength(10);
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("");
    expect(columnsButton()).toHaveTextContent(/^Columns ▾$/);

    await click(within(board()).getByRole("checkbox", { name: "In Progress" }));
    expect(heads()).not.toContain("In Progress 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("In Progress");
  });

  it("keeps the browser's pick for a fresh Board with no hide in the address", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    await click(within(board()).getByRole("checkbox", { name: "Draft" }));
    unmount();

    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(heads()).toEqual(["Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0", "Done 1"]);
  });

  it("lets the address's hidden columns win over the browser's pick", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    unmount();

    await renderApp(fake, "/?view=board&project=acme&hide=Draft,Ready,Accepted");
    expect(heads()).toEqual(["In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Done 1", "Cancelled 1"]);
  });

  it("shows every column for an empty hide=", async () => {
    await renderApp(await world(), "/?view=board&project=acme&hide=");
    loaded();
    expect(heads()).toHaveLength(10);
    expect(heads()).toContain("Done 1");
    expect(heads()).toContain("Cancelled 1");
  });

  it("follows the Projects picker, keeping the address", async () => {
    await renderApp(await world(), "/?view=board");
    loaded();
    expect(titles("In Progress")).toEqual(["Beta export", "Sign-up page"]);
    const picker = within(board()).getByRole("combobox", { name: "Projects" });
    await userEvent.selectOptions(picker, "beta");
    await settled();
    expect(window.location.search).toBe("?view=board&project=beta");
    expect(titles("In Progress")).toEqual(["Beta export"]);
    expect(cards("Ready")).toEqual([]);
  });
});

describe("the Board's cards", () => {
  it("show the ID, kind, Blocked, the needs-you dot and the Spec's kind and name", async () => {
    const fake = await world();
    await fake.s.askRound("acme", fake.ids.signup);
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    const reset = card("Password reset");
    expect(within(reset).getByRole("button", { name: fake.ids.reset })).toHaveTextContent("#6");
    expect(reset).toHaveTextContent("Prototype");
    expect(reset).toHaveTextContent("⛔ Blocked");
    expect(within(reset).queryByRole("img", { name: "Needs you" })).toBeNull();
    expect(reset).toHaveTextContent(/Spec\s*Login$/);
    expect(within(card("Sign-up page")).getByRole("img", { name: "Needs you" })).toBeInTheDocument();

    const form = card("Login form");
    expect(within(form).getByRole("button", { name: fake.ids.form })).toBeInTheDocument();
    expect(form).not.toHaveTextContent("Prototype");
    expect(form).not.toHaveTextContent("Blocked");
    expect(within(form).queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("show the CI badge of a not-Done Ticket, none for no CI result or a Done Ticket, and follow the live feed", async () => {
    const fake = await world();
    const { s, ids } = fake;
    // pushed makes an Accepted Ticket whose commit's CI is outcome in run.
    const pushed = async (title: string, n: number, outcome: "running" | "failed" | "passed") => {
      const id = await s.ticket("acme", { title, spec: ids.spec });
      await s.accepted("acme", id, { branch: `b${n}`, commit: `c${n}` });
      await s.pushed("acme", id, `c${n}`);
      await s.ci("acme", id, outcome, `c${n}`, n);
      return id;
    };
    const running = await pushed("Checks running", 1, "running");
    await pushed("Checks failed", 2, "failed");
    const shipped = await pushed("Shipped", 3, "passed");
    await s.human(`${s.ticketPath("acme", shipped)}/land`, { commit: "c3" });
    await renderApp(fake, "/?view=board&project=acme&hide=Cancelled");
    loaded();
    expect(card("Checks running")).toHaveTextContent("CI running");
    expect(card("Checks failed")).toHaveTextContent("CI failed");
    expect(card("Password reset")).not.toHaveTextContent("CI");
    expect(card("Shipped")).not.toHaveTextContent("CI");
    await s.ci("acme", running, "passed", "c1", 1);
    expect(card("Checks running")).toHaveTextContent("CI passed");
  });

  it("show full IDs on All Projects, and the Spec every Ticket is on", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board");
    loaded();
    expect(within(card("Login form")).getByRole("button", { name: fake.ids.form })).toHaveTextContent(fake.ids.form);
    expect(card("Beta export")).toHaveTextContent(new RegExp(`^${fake.ids.beta}\\s*Beta export\\s*Spec\\s*A Spec$`));
  });

  it("are newest first by creation", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(titles("Ready")).toEqual(["Password reset", "Login form"]);
  });

  it("move column when a Ticket changes State, and pulse then but not on first show", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(within(board()).getAllByRole("link").filter((c) => c.hasAttribute("data-pulsing"))).toEqual([]);

    fake.w.advance(30 * MINUTE);
    await fake.s.claimFor("acme", fake.ids.form);
    expect(titles("In Progress")).toEqual(["Sign-up page", "Login form"]);
    expect(titles("Ready")).toEqual(["Password reset"]);
    expect(card("Login form")).toHaveAttribute("data-pulsing", "true");
    expect(card("Sign-up page")).not.toHaveAttribute("data-pulsing");
  });

  it("pulse when a Ticket changes in place", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();

    fake.w.advance(30 * MINUTE);
    await fake.w.api.act("acme", fake.ids.signup, "edit_ticket", { title: "Sign-up page, redone" });
    await settled();
    expect(card("Sign-up page, redone")).toHaveAttribute("data-pulsing", "true");
    expect(card("Login form")).not.toHaveAttribute("data-pulsing");
  });

  it("don't pulse when a hidden column is shown", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(card("Old work")).not.toHaveAttribute("data-pulsing");
  });

  it("link to the Board's own address with their popup, not into the Inbox", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme&group=spec&hide=");
    loaded();
    expect(card("Login form")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=&popup=${fake.ids.form}`);
    expect(card("Old work")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=&popup=${fake.ids.old}`);
  });
});

describe("the Board's Ticket popup", () => {
  const popup = () => screen.getByRole("dialog", { name: "Ticket popup" });
  // popupHeading is the Ticket popup's heading for the Ticket titled title.
  const popupHeading = (title: string) => within(popup()).getByRole("heading", { name: new RegExp(title) });
  const openCard = async (title: string) => {
    await click(within(card(title)).getByText(title));
    return popupHeading(title);
  };

  it("opens from a card that doesn't need the human, with the Ticket's pane and actions, in the address", async () => {
    const fake = await world();
    const idea = await fake.s.ticket("acme", { title: "Rough idea", state: "Draft" });
    await renderApp(fake, "/?view=board&project=acme&hide=Done");
    loaded();
    expect(await openCard("Rough idea")).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&hide=Done&popup=${idea}`);
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
    const fake = await world();
    let fail: ((e: Error) => void) | undefined;
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.ids.form}`, { getTicket: () => new Promise((_, reject) => (fail = reject)) });
    loaded();
    expect(popup()).toHaveTextContent("Loading…");
    expect(fail).toBeDefined();
    await act(async () => fail!(new ApiError(404, "not_found", `Ticket ${fake.ids.form} not found`)));
    await settled();
    expect(within(popup()).getByRole("alert")).toHaveTextContent(`Ticket ${fake.ids.form} not found`);
  });

  it("offers no edits for a Ticket of an Archived Project", async () => {
    const fake = await world();
    await fake.s.project("old", "Old");
    const legacy = await fake.s.ticket("old", { title: "Legacy", state: "Done" });
    await fake.w.api.archiveProject("old");
    await renderApp(fake, `/?view=board&project=old&popup=${legacy}`);
    expect(popupHeading("Legacy")).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("follows the live feed", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await openCard("Login form");
    await fake.w.api.act("acme", fake.ids.form, "edit_ticket", { title: "Login form, redone" });
    await settled();
    expect(within(popup()).getByRole("heading", { name: /Login form, redone/ })).toBeInTheDocument();
  });

  it("closes once its Move button has moved the Ticket", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme&hide=Done");
    loaded();
    await openCard("Login form");
    const before = (await fake.w.api.getTicket("acme", fake.ids.form)).state;
    await click(within(popup()).getByRole("button", { name: "Move" }));
    await settled();
    expect((await fake.w.api.getTicket("acme", fake.ids.form)).state).not.toBe(before);
    noPopup();
  });

  it("stays a Ticket popup when its Ticket starts needing the human", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await openCard("Login form");
    await fake.s.askRound("acme", fake.ids.form);
    expect(within(board()).getByRole("img", { name: "Needs you" })).toBeInTheDocument();
    expect(within(popup()).getByRole("heading", { name: /Login form/ })).toBeInTheDocument();
  });

  it("switches to its Spec's popup address from the Spec link", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme&group=spec");
    loaded();
    await openCard("Login form");
    await click(within(popup()).getByRole("button", { name: `${fake.ids.spec} Login` }));
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&popup=${fake.ids.spec}`);
  });

  it.each([
    ["×", () => click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => press("{Escape}")],
    ["the backdrop", () => click(popup().parentElement!)],
  ])("closes on %s, pushing the Board's address without popup and keeping its grouping and hidden columns", async (_, close) => {
    await renderApp(await world(), "/?view=board&project=acme&group=spec&hide=Draft");
    loaded();
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
    await renderApp(await world(), "/?view=board&project=acme&hide=Draft");
    loaded();
    await openCard("Login form");
    await travel(() => window.history.back());
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme&hide=Draft");
    expect(card("Login form")).toHaveFocus();

    await travel(() => window.history.forward());
    expect(popupHeading("Login form")).toBeInTheDocument();
  });

  it("reopens when the address with popup= is loaded", async () => {
    const fake = await world();
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.ids.signup}`);
    expect(popupHeading("Sign-up page")).toBeInTheDocument();
  });

  it("opens on any Project's Ticket, whatever the picker is on", async () => {
    const fake = await world();
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.ids.beta}`);
    expect(popupHeading("Beta export")).toBeInTheDocument();
  });

  it("shows the read's error for an unknown ID, and leaves the address alone", async () => {
    await renderApp(await world(), "/?view=board&project=acme&popup=acme-99");
    loaded();
    expect(within(popup()).getByRole("alert")).toHaveTextContent("there is no Ticket acme-99");
    expect(window.location.search).toBe("?view=board&project=acme&popup=acme-99");
  });

  it("shows nothing while the Inbox data loads", async () => {
    const fake = await world();
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    const needsMe = fake.w.api.needsMe.bind(fake.w.api);
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.ids.form}`, {
      needsMe: async (...args) => {
        await gate;
        return needsMe(...args);
      },
    });
    expect(within(board()).getByText("Loading…")).toBeInTheDocument();
    noPopup();
    await act(async () => release());
    await settled();
    expect(screen.getByRole("dialog", { name: "Ticket popup" })).toBeInTheDocument();
  });
});

describe("the Board's Focus popup", () => {
  const popup = () => screen.getByRole("dialog", { name: "Focus popup" });
  // item and question look the popup up again each try, so a popup
  // replaced isn't held on to.
  const item = () => within(popup()).getByRole("article", { name: "Focus Item" });
  // question is the popup's heading for the Question titled title; each
  // Question is a new Focus Item, so it looks in the popup, not an item.
  const question = (title: string) => within(popup()).getByRole("heading", { name: title });

  const QUESTIONS: QuestionSeed[] = [
    { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
    { title: "Which font?", body: "Serif or sans" },
    { title: "Which size?", body: "S or M" },
  ];

  // asking is world() with Pick a colour asking Round 1, its three Questions
  // open, and Sign-up page In Review, both in Needs Me.
  async function asking() {
    const fake = await world();
    const pick = await fake.s.ticket("acme", { title: "Pick a colour", spec: fake.ids.spec });
    await fake.s.askRound("acme", pick, QUESTIONS);
    await fake.s.inReview("acme", fake.ids.signup, WORK, "Added the sign-up page");
    return { ...fake, pick };
  }

  it("opens from a card that needs the human, with its Focus Item and no Skip, N left or Open in Inbox", async () => {
    const fake = await asking();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(within(card("Pick a colour")).getByText("Pick a colour"));
    expect(question("Which colour?")).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&popup=${fake.pick}`);
    expect(within(popup()).getByText("Question 1 of 3 · Round 1")).toBeInTheDocument();
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(within(popup()).queryByRole("button", { name: "Skip" })).toBeNull();
    expect(within(popup()).queryByRole("link", { name: /Open in Inbox/ })).toBeNull();
    expect(within(popup()).queryByRole("button", { name: /New/ })).toBeNull();
    expect(popup()).not.toHaveTextContent(/\d+ left/);
    // Only the one Ticket's work: not Sign-up page's In Review.
    expect(popup()).not.toHaveTextContent("Sign-up page");
  });

  it("shows Loading… until the Round is read", async () => {
    const fake = await asking();
    let release: () => void = () => {};
    const gate = new Promise<void>((r) => (release = r));
    const getTicket = fake.w.api.getTicket.bind(fake.w.api);
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.pick}`, {
      getTicket: async (...args) => {
        await gate;
        return getTicket(...args);
      },
    });
    loaded();
    expect(within(item()).getByText("Pick a colour")).toBeInTheDocument();
    expect(popup()).toHaveTextContent("Loading…");
    expect(within(popup()).queryByRole("region", { name: "Question" })).toBeNull();
    await act(async () => release());
    await settled();
    expect(question("Which colour?")).toBeInTheDocument();
  });

  it("shows the Round's Questions one at a time, goes on to the Ticket's next Focus Item, and closes once it needs nothing more", async () => {
    const fake = await asking();
    await renderApp(fake, "/?view=board&project=acme&hide=Draft");
    loaded();
    await click(within(card("Pick a colour")).getByText("Pick a colour"));
    question("Which colour?");
    expect(within(popup()).getByText("Question 1 of 3 · Round 1")).toBeInTheDocument();
    expect(within(popup()).queryByText("Which font?")).toBeNull();
    await click(within(popup()).getByRole("button", { name: "Answer" }));
    expect(question("Which font?")).toBeInTheDocument();
    expect(within(popup()).getByText("Question 2 of 3 · Round 1")).toBeInTheDocument();
    await userEvent.type(within(popup()).getByRole("textbox"), "Sans");
    await click(within(popup()).getByRole("button", { name: "Answer" }));
    expect(question("Which size?")).toBeInTheDocument();
    await click(within(popup()).getByRole("button", { name: "Dismiss" }));

    // The Round is answered: its answers wait for the agent, which the
    // human can end with Release.
    const resume = within(item()).getByRole("region", { name: "Resume or release" });
    expect((await fake.w.api.getTicket("acme", fake.pick)).rounds![0].questions.map((q) => [q.status, q.answer])).toEqual([
      ["answered", "Blue"],
      ["answered", "Sans"],
      ["dismissed", undefined],
    ]);
    await click(within(resume).getByRole("button", { name: "Release…" }));
    await click(within(resume).getByRole("button", { name: "Release Claim" }));
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme&hide=Draft");
    expect(heads()[0]).toBe("Ready 3");
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("shows Loading… while Needs Me still lists a read Round with no open Question, and closes once it catches up", async () => {
    const fake = await asking();
    // Needs Me as the Round was asked, before every Question is dismissed.
    const stale = await fake.w.api.needsMe();
    for (const q of [1, 2, 3]) await fake.w.api.act("acme", fake.pick, "dismiss_question", { question: q });
    let behind = true;
    const needsMe = fake.w.api.needsMe.bind(fake.w.api);
    const getTicket = fake.w.api.getTicket.bind(fake.w.api);
    const read = vi.fn(getTicket);
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.pick}`, { needsMe: async () => (behind ? stale : needsMe()), getTicket: read });
    loaded();
    // Once read, the Round has nothing to show, so no Focus Item either.
    expect(within(popup()).queryByRole("article", { name: "Focus Item" })).toBeNull();
    expect(popup()).toHaveTextContent("Loading…");
    // It waits on that read, rather than reading the Round again and again.
    // eslint-disable-next-line no-restricted-syntax -- nothing happens to wait for: the test shows the read is not repeated
    await act(() => new Promise((r) => setTimeout(r, 50)));
    expect(read.mock.calls.length).toBeLessThan(5);

    behind = false;
    await fake.s.pickUpAnswers("acme", fake.pick);
    noPopup();
  });

  it("closes quietly when the agent deals with its Ticket", async () => {
    const fake = await asking();
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.pick}`);
    question("Which colour?");
    const length = window.history.length;
    await fake.w.api.act("acme", fake.pick, "move", { to_state: "In Progress" });
    await settled();
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme");
    expect(window.history.length).toBe(length + 1);
    expect(screen.queryByText(/Dealt with elsewhere/)).toBeNull();
  });

  it("opens for a Ticket listed in a CI item, with that item, whose options act on every Ticket it lists", async () => {
    // Basket and Checkout Accepted, pushed together, whose CI failed: one
    // CI failed item listing both.
    const fake = await world();
    const basket = await fake.s.ticket("acme", { title: "Basket", state: "Accepted", spec: fake.ids.spec });
    const checkout = await fake.s.ticket("acme", { title: "Checkout", state: "Accepted", spec: fake.ids.spec });
    await failCI(fake.s, "acme", [basket, checkout]);
    await renderApp(fake, `/?view=board&project=acme&popup=${checkout}`);
    const ciItem = item();
    expect(within(ciItem).getByRole("heading", { level: 2 })).toHaveTextContent("CI failed at abc1234");
    const panel = within(ciItem).getByRole("region", { name: "CI failed" });
    await click(within(within(panel).getByRole("group", { name: "Failure options" })).getByRole("button", { name: "Land anyway" }));
    for (const id of [basket, checkout]) expect((await fake.w.api.getTicket("acme", id)).land_anyway).toBe(true);
    noPopup();
  });

  it("reopens as a Focus popup when the address with popup= is loaded for a Ticket that needs the human", async () => {
    const fake = await asking();
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.pick}`);
    expect(question("Which colour?")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
  });

  it("still opens from a card, with its item and needs-you dot, when its Project's Focus Kinds leave the reason off", async () => {
    const fake = await world();
    await fake.s.askRound("acme", fake.ids.signup);
    await fake.s.answerRound("acme", fake.ids.signup);
    await fake.w.api.setFocusKinds("acme", ["answered"]);
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(within(card("Sign-up page")).getByRole("img", { name: "Needs you" })).toBeInTheDocument();
    await click(within(card("Sign-up page")).getByText("Sign-up page"));
    expect(within(item()).getByRole("heading", { name: "Sign-up page" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&popup=${fake.ids.signup}`);
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
  });

  it("gives focus back to the card when it closes", async () => {
    await renderApp(await asking(), "/?view=board&project=acme");
    loaded();
    await click(within(card("Pick a colour")).getByText("Pick a colour"));
    question("Which colour?");
    await click(within(popup()).getByRole("button", { name: "Close" }));
    noPopup();
    expect(window.location.search).toBe("?view=board&project=acme");
    expect(card("Pick a colour")).toHaveFocus();
  });
});

describe("the Board's grouping", () => {
  it("is By Ticket by default, and shares its pick with the State lists", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    const toggle = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(toggle).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
    await click(within(toggle).getByRole("button", { name: "Spec" }));
    expect(within(toggle).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    expect(new URLSearchParams(window.location.search).get("group")).toBe("spec");
    unmount();

    const inbox = await renderApp(fake, "/?view=inbox&project=acme&state=Ready");
    const list = within(screen.getByRole("main", { name: "Tickets" })).getByRole("group", { name: "Group the list" });
    expect(within(list).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    await click(within(list).getByRole("button", { name: "Ticket" }));
    inbox.unmount();

    await renderApp(fake, "/?view=board&project=acme");
    const again = within(board()).getByRole("group", { name: "Group the board" });
    expect(within(again).getByRole("button", { name: "Ticket" })).toHaveAttribute("aria-pressed", "true");
  });
});

describe("the Board's By Spec lanes", () => {
  // lanesWorld makes a second Spec, newer than Login, with a Ready and an In Progress Ticket,
  // and a third whose only Ticket is Done.
  async function lanesWorld() {
    const fake = await world();
    const { s, w } = fake;
    const billing = await s.spec("acme", { title: "Billing" }); // acme-7
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Invoice list", state: "Ready", spec: billing }); // acme-8
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Refunds", state: "In Progress", spec: billing }); // acme-9
    w.advance(MINUTE);
    const archive = await s.spec("acme", { title: "Archive" }); // acme-10
    w.advance(MINUTE);
    await s.ticket("acme", { title: "Cold storage", state: "Done", spec: archive }); // acme-11
    return fake;
  }
  const lanes = () => within(board()).getAllByRole("group", { name: /^Lane: / }).map((l) => l.getAttribute("aria-label"));
  const lane = (title: string) => within(board()).getByRole("group", { name: `Lane: ${title}` });
  const bySpec = async (url = "/?view=board&project=acme&group=spec") => renderApp(await lanesWorld(), url);

  it("shows one lane per Spec with a Ticket in a shown column, newest Spec first, and no lanes By Ticket", async () => {
    await bySpec();
    loaded();
    expect(lanes()).toEqual(["Lane: Billing", "Lane: Login"]);
    await click(within(within(board()).getByRole("group", { name: "Group the board" })).getByRole("button", { name: "Ticket" }));
    expect(within(board()).queryAllByRole("group", { name: /^Lane: / })).toEqual([]);
    expect(cards("Ready")).toHaveLength(3);
  });

  it("adds a lane when its Spec's Done Tickets are shown with Columns ▾", async () => {
    await bySpec("/?view=board&project=acme&group=spec&hide=");
    loaded();
    expect(lanes()).toEqual(["Lane: Archive", "Lane: Billing", "Lane: Login"]);
  });

  it("heads each lane with its Lane header, which opens the Spec's popup", async () => {
    await bySpec();
    loaded();
    const row = within(lane("Login")).getByRole("region", { name: "Login" });
    // One line: the count of the Tickets the lane shows, and no State chips.
    expect(within(row).getByText("3 shown")).toBeInTheDocument();
    expect(row.querySelector(".state-chips, .strip")).toBeNull();
    expect(within(row).getByRole("link")).toHaveAttribute("href", "?view=board&project=acme&group=spec&popup=acme-1");
    await click(within(row).getByRole("link"));
    expect(screen.getByRole("dialog", { name: "Spec popup" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=board&project=acme&group=spec&popup=acme-1");
  });

  it("puts each Ticket's card in its lane's column, without the Spec line", async () => {
    await bySpec();
    loaded();
    const ready = within(lane("Login")).getByRole("region", { name: "Ready" });
    expect(within(ready).getAllByRole("link").map((c) => c.querySelector(".board-card-title")?.textContent)).toEqual(["Password reset", "Login form"]);
    const billingReady = within(lane("Billing")).getByRole("region", { name: "Ready" });
    expect(within(billingReady).getByRole("link", { name: /Invoice list/ })).not.toHaveTextContent("Billing");
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ }).querySelector(".board-card-spec")).toBeNull();
  });

  it("counts a column's Tickets across all lanes in its head", async () => {
    await bySpec();
    loaded();
    expect(heads()).toContain("Ready 3");
    expect(heads()).toContain("In Progress 2");
    expect(heads()).toContain("Draft 0");
  });

  it("collapses a lane to its header, keeping its per-column counts and the head totals", async () => {
    await bySpec();
    loaded();
    const toggle = within(lane("Login")).getByRole("button", { name: "Login" });
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Login")).queryAllByRole("link", { name: /Login form|Sign-up page|Password reset/ })).toEqual([]);
    expect(within(within(lane("Login")).getByRole("region", { name: "Ready" })).getByText("2")).toBeInTheDocument();
    expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByText("1")).toBeInTheDocument();
    expect(within(lane("Billing")).getByRole("link", { name: /Invoice list/ })).toBeInTheDocument();
    expect(heads()).toContain("Ready 3");
    await click(toggle);
    expect(within(lane("Login")).getByRole("link", { name: /Login form/ })).toBeInTheDocument();
  });

  it("keeps folded lanes across a card's popup opening and closing", async () => {
    await bySpec();
    loaded();
    await click(within(lane("Billing")).getByRole("button", { name: "Billing" }));
    await click(within(lane("Login")).getByText("Login form"));
    screen.getByRole("dialog", { name: "Ticket popup" });
    await press("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(lane("Billing")).getByRole("button", { name: "Billing" })).toHaveAttribute("aria-expanded", "false");
    expect(within(lane("Billing")).queryByRole("link", { name: /Invoice list/ })).toBeNull();
  });

  it("follows a Ticket moving column inside its lane", async () => {
    const fake = await lanesWorld();
    await renderApp(fake, "/?view=board&project=acme&group=spec");
    loaded();
    await fake.s.claimFor("acme", fake.ids.form);
    expect(within(within(lane("Login")).getByRole("region", { name: "In Progress" })).getByRole("link", { name: /Login form/ })).toBeInTheDocument();
    expect(heads()).toContain("In Progress 3");
  });
});

describe("the Board's Spec popup", () => {
  const popup = () => screen.getByRole("dialog", { name: "Spec popup" });
  const noPopup = () => expect(screen.queryByRole("dialog")).toBeNull();
  const lane = (title: string) => within(board()).getByRole("group", { name: `Lane: ${title}` });
  const header = (title: string) => within(within(lane(title)).getByRole("region", { name: title })).getByRole("link");
  const openLane = async (title: string) => {
    await click(header(title));
    return popup();
  };
  const bySpec = "/?view=board&project=acme&group=spec&hide=Done";
  const specPopup = popup;

  // finishedMap makes a Map whose every Ticket is Done: acme-1, its Ticket acme-2.
  async function finishedMap() {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const body = ["Plan the year", "## Destination", "A plan.", "## Decisions so far", "## Not yet specified", "## Out of scope"].join("\n\n");
    const map = await w.api.submitMap("acme", body);
    await s.accepted("acme", map.charting.id);
    await s.human(`${s.ticketPath("acme", map.charting.id)}/land`, { commit: WORK.commit });
    return { w, s, map: map.spec.id };
  }

  it("opens from a Lane header with the Spec's pane and its Tickets, and its chevron only folds", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    const chevron = () => within(lane("Login")).getByRole("button", { name: "Login" });
    await click(chevron());
    noPopup();
    expect(chevron()).toHaveAttribute("aria-expanded", "false");
    await click(chevron());
    noPopup();
    await openLane("Login");
    expect(within(popup()).getByRole("region", { name: "Spec" })).toBeInTheDocument();
    expect(within(popup()).getByRole("heading", { name: "Login" })).toBeInTheDocument();
    expect(within(popup()).getByRole("button", { name: "Close" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
    // The lane stays as it was behind it.
    expect(chevron()).toHaveAttribute("aria-expanded", "true");
  });

  it("leaves a modified click on a Lane header to the browser", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    expect(header("Login")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
    fireEvent.click(header("Login"), { metaKey: true });
    fireEvent.click(header("Login"), { button: 1 });
    noPopup();
    expect(window.location.search).toBe(bySpec.slice(1));
  });

  it("shows a Requirement's pane even when one of its Stage Tickets needs the human", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec, stages } = await w.api.submitRequirement("acme", "Offline mode");
    await s.askRound("acme", stages[0].id);
    await renderApp({ w, s } as Fake, "/?view=board&project=acme&group=spec");
    within(board()).getByRole("group", { name: "Lane: Offline mode" });
    await click(header("Offline mode"));
    expect(within(specPopup()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&popup=${spec.id}`);
  });

  it("shows a Map's pane", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec } = await w.api.submitMap("acme", "Plan the year");
    await renderApp({ w, s } as Fake, "/?view=board&project=acme&group=spec");
    within(board()).getByRole("group", { name: "Lane: Plan the year" });
    await click(header("Plan the year"));
    const dialog = specPopup();
    expect(within(dialog).getByRole("region", { name: "Map" })).toBeInTheDocument();
    expect(within(dialog).getByRole("heading", { name: "Plan the year", level: 2 })).toBeInTheDocument();
    expect(window.location.search).toContain(`popup=${spec.id}`);
  });

  it("switches to a Ticket's popup when a Ticket is chosen in it, and Back returns to the Spec popup", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    await openLane("Login");
    await click(within(popup()).getByRole("button", { name: /Login form/ }));
    const ticket = screen.getByRole("dialog", { name: "Ticket popup" });
    expect(within(ticket).getByRole("heading", { name: /Login form/ })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Spec popup" })).toBeNull();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.form}`);
    await travel(() => window.history.back());
    expect(specPopup()).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
    // No Ticket shows as selected.
    expect(popup().querySelector("[aria-current]")).toBeNull();
  });

  it("switches to the Spec's popup from a Ticket popup's Spec link", async () => {
    const fake = await world();
    await renderApp(fake, bySpec);
    loaded();
    await click(within(card("Login form")).getByText("Login form"));
    screen.getByRole("dialog", { name: "Ticket popup" });
    await click(screen.getByRole("button", { name: `${fake.ids.spec} Login` }));
    expect(specPopup()).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&hide=Done&popup=${fake.ids.spec}`);
  });

  it("switches to the Map's popup from a Requirement made from it", async () => {
    const fake = await finishedMap();
    const { spec } = await fake.w.api.submitRequirement("acme", "Offline mode", fake.map); // acme-3
    await renderApp(fake as unknown as Fake, `/?view=board&project=acme&group=spec&popup=${spec.id}`);
    await click(within(specPopup()).getByRole("link", { name: /^Map/ }));
    expect(within(specPopup()).getByRole("region", { name: "Map" })).toBeInTheDocument();
    expect(window.location.search).toBe(`?view=board&project=acme&group=spec&popup=${fake.map}`);
  });

  it("switches to the new Requirement's popup after Write the Spec", async () => {
    const fake = await finishedMap();
    await renderApp(fake as unknown as Fake, `/?view=board&project=acme&group=spec&popup=${fake.map}`);
    await click(within(specPopup()).getByRole("button", { name: "Write the Spec" }));
    await click(within(screen.getByRole("dialog", { name: "Write the Spec" })).getByRole("button", { name: "Submit requirement" }));
    expect(window.location.search).toContain("popup=acme-3");
    expect(within(specPopup()).getByRole("region", { name: "Requirement" })).toBeInTheDocument();
  });

  it.each([
    ["×", () => click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => press("{Escape}")],
    ["the backdrop", () => click(popup().parentElement!)],
    ["Back", () => travel(() => window.history.back())],
  ])("closes on %s, keeping the grouping, hidden columns and folded lanes", async (_, close) => {
    await renderApp(await world(), bySpec);
    loaded();
    await click(within(lane("Login")).getByRole("button", { name: "Login" }));
    await openLane("Login");
    await close();
    noPopup();
    expect(window.location.search).toBe(bySpec.slice(1));
    expect(within(lane("Login")).getByRole("button", { name: "Login" })).toHaveAttribute("aria-expanded", "false");
    expect(within(board()).queryByRole("region", { name: "Done" })).toBeNull();
  });
});

describe("+ New ▾ on the Board", () => {
  const url = "/?view=board&project=acme";
  const kinds = [
    ["Requirement", "New requirement", "Submit requirement"],
    ["Map", "New Map", "Submit map"],
    ["Small Change", "New Small Change", "Submit small change"],
  ] as const;
  // create picks kind from + New ▾ and submits some text.
  async function create(kind: string, dialog: string, button: string) {
    await click(within(board()).getByRole("button", { name: "More to create" }));
    await click(within(screen.getByRole("menu")).getByRole("menuitem", { name: kind }));
    const modal = screen.getByRole("dialog", { name: dialog });
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await click(within(modal).getByRole("button", { name: button }));
  }

  it.each(kinds)("stays on the Board after a %s, saying so, with nothing opened", async (kind, dialog, button) => {
    await renderApp(await world(), url);
    loaded();
    await create(kind, dialog, button);

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(within(board()).getByRole("status")).toHaveTextContent(`${kind} added`);
    expect(window.location.search).toBe("?view=board&project=acme");
  });

  it("keeps the columns, grouping and folded lanes", async () => {
    const start = "/?view=board&project=acme&group=spec&hide=Done&needs=1";
    await renderApp(await world(), start);
    loaded();
    const login = () => within(within(board()).getByRole("group", { name: "Lane: Login" })).getByRole("button", { name: "Login" });
    await click(login());
    await create("Small Change", "New Small Change", "Submit small change");

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(window.location.search).toBe(start.slice(1));
    expect(login()).toHaveAttribute("aria-expanded", "false");
  });

  describe("the note", () => {
    beforeEach(() => vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true, advanceTimeDelta: 1 }));
    afterEach(() => vi.useRealTimers());

    it("goes after 4 s, and a second submit's note replaces the first and restarts the 4 s", async () => {
      await renderApp(await world(), url);
      loaded();
      await create("Requirement", "New requirement", "Submit requirement");
      expect(within(board()).getByRole("status")).toHaveTextContent("Requirement added");

      act(() => vi.advanceTimersByTime(3000));
      await create("Small Change", "New Small Change", "Submit small change");
      expect(within(board()).getByRole("status")).toHaveTextContent("Small Change added");
      expect(within(board()).getAllByRole("status")).toHaveLength(1);

      act(() => vi.advanceTimersByTime(3000));
      expect(within(board()).getByRole("status")).toHaveTextContent("Small Change added");
      act(() => vi.advanceTimersByTime(1500));
      expect(within(board()).queryByRole("status")).toBeNull();
    });
  });
});

describe("the Board's Needs Me column", () => {
  const needsOn = "/?view=board&project=acme&needs=1";
  const needsCol = () => within(board()).getByRole("region", { name: "Needs Me" });
  const needsTitles = () => within(needsCol()).queryAllByRole("link").map((c) => c.querySelector(".board-card-title")?.textContent);

  it("is off by default: no column, the address untouched, and its checkbox after In Progress", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(heads()).not.toContain("Needs Me 2");
    expect(within(board()).queryByRole("region", { name: "Needs Me" })).toBeNull();
    expect(window.location.search).toBe("?view=board&project=acme");
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
  });

  it("is shown with Columns ▾, kept in the address and the browser, and doesn't change the N/M count", async () => {
    const fake = await world();
    await needing(fake);
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(new URLSearchParams(window.location.search).get("needs")).toBe("1");
    expect(heads()).toEqual(["Draft 0", "Ready 1", "In Progress 0", "Needs Me 2", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0"]);
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
    unmount();

    await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads()).toContain("Needs Me 2");

    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(new URLSearchParams(window.location.search).get("needs")).toBe("0");
    expect(heads()).not.toContain("Needs Me 2");
  });

  it("follows the address over the browser, and old hide= addresses open without it", async () => {
    localStorage.setItem("code-central.board.needs", "1");
    const fake = await world();
    await needing(fake);
    const { unmount } = await renderApp(fake, "/?view=board&project=acme&needs=0");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads().some((h) => h?.startsWith("Needs Me"))).toBe(false);
    unmount();
    localStorage.clear();
    await renderApp(fake, "/?view=board&project=acme&hide=Done");
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(heads().some((h) => h?.startsWith("Needs Me"))).toBe(false);
  });

  it("holds needing Tickets in Needs Me's order, out of their State columns, with State chip and reason", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, needsOn);
    within(board()).getByRole("link", { name: /Password reset/ });
    expect(needsTitles()).toEqual(["Sign-up page", "Login form"]);
    expect(cards("Ready").map((c) => c.textContent)).not.toContain("Login form");
    expect(titles("Ready")).toEqual(["Password reset"]);
    const [first, second] = within(needsCol()).getAllByRole("link");
    expect(first).toHaveTextContent("Needs Answer");
    expect(first).toHaveTextContent("Questions");
    expect(second).toHaveTextContent("In Review");
    expect(second).toHaveTextContent("In Review");
    expect(within(column("Ready")).getByRole("link")).not.toHaveTextContent("Questions");
    expect(within(board()).getByRole("heading", { name: "Needs Me 2" })).toBeInTheDocument();
  });

  it("lists a CI item's Tickets, and not Base diverged", async () => {
    const fake = await world();
    const basket = await fake.s.ticket("acme", { title: "Basket", state: "Accepted", spec: fake.ids.spec });
    await failCI(fake.s, "acme", [basket]);
    await renderApp(fake, needsOn);
    loaded();
    expect(needsTitles()).toEqual(["Basket"]);
    expect(within(needsCol()).getByRole("link")).toHaveTextContent("CI failed");
  });

  it("shows a needing Ticket whose State column is hidden, and no other Ticket of it", async () => {
    const fake = await world();
    await fake.s.inReview("acme", fake.ids.form);
    await renderApp(fake, `${needsOn}&hide=Done,Cancelled,Ready,In Review`);
    within(board()).getByRole("link", { name: /Login form/ });
    expect(needsTitles()).toEqual(["Login form"]);
    expect(within(board()).queryByRole("region", { name: "Ready" })).toBeNull();
    expect(within(board()).queryByRole("link", { name: /Dropped/ })).toBeNull();
  });

  it("follows the Projects picker", async () => {
    const fake = await world();
    await fake.s.askRound("acme", fake.ids.signup);
    await fake.s.askRound("beta", fake.ids.beta);
    await renderApp(fake, "/?view=board&needs=1");
    within(board()).getByRole("link", { name: /Beta export/ });
    expect(needsTitles()).toEqual(["Sign-up page", "Beta export"]);
    await userEvent.selectOptions(within(board()).getByRole("combobox", { name: "Projects" }), "acme");
    await settled();
    expect(needsTitles()).toEqual(["Sign-up page"]);
  });

  it("reads Nothing needs you when empty", async () => {
    await renderApp(await world(), needsOn);
    loaded();
    expect(needsCol()).toHaveTextContent("Nothing needs you");
  });

  it("opens a card's Focus popup on click", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, needsOn);
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(within(needsCol()).getByRole("link", { name: /Sign-up page/ }));
    expect(screen.getByRole("dialog", { name: "Focus popup" })).toBeInTheDocument();
  });

  it("moves a card in and out live, pulsing on joining even with no Ticket update", async () => {
    const fake = await world();
    await renderApp(fake, needsOn);
    loaded();
    expect(needsTitles()).toEqual([]);
    await fake.s.askRound("acme", fake.ids.signup);
    expect(needsTitles()).toEqual(["Sign-up page"]);
    expect(within(needsCol()).getByRole("link")).toHaveAttribute("data-pulsing", "true");
    expect(titles("In Progress")).toEqual([]);

    await fake.s.answerRound("acme", fake.ids.signup);
    await fake.s.pickUpAnswers("acme", fake.ids.signup);
    expect(titles("In Progress")).toEqual(["Sign-up page"]);
    expect(needsTitles()).toEqual([]);
  });

  it("doesn't pulse when the column is just shown", async () => {
    const fake = await world();
    await needing(fake);
    await renderApp(fake, "/?view=board&project=acme");
    within(board()).getByRole("link", { name: /Password reset/ });
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Needs Me" }));
    expect(within(needsCol()).getAllByRole("link").filter((c) => c.hasAttribute("data-pulsing"))).toEqual([]);
  });

  describe("By Spec", () => {
    it("has a cell per lane, a collapsed lane's count, and a lane for a Spec with only a hidden needing Ticket", async () => {
      const fake = await world();
      await fake.s.askRound("acme", fake.ids.signup);
      const archive = await fake.s.spec("acme", { title: "Archive" });
      fake.w.advance(MINUTE);
      const cold = await fake.s.ticket("acme", { title: "Cold storage", state: "Ready", spec: archive });
      await fake.s.inReview("acme", cold);
      await renderApp(fake, `${needsOn}&group=spec&hide=Done,Cancelled,In Review`);
      within(board()).getByRole("group", { name: "Lane: Login" });
      const lanes = within(board()).getAllByRole("group", { name: /^Lane: / });
      expect(lanes.map((l) => l.getAttribute("aria-label"))).toEqual(["Lane: Archive", "Lane: Login"]);
      const login = within(board()).getByRole("group", { name: "Lane: Login" });
      expect(within(within(login).getByRole("region", { name: "Needs Me" })).getAllByRole("link")).toHaveLength(1);
      await click(within(login).getByRole("button", { name: "Login" }));
      expect(within(within(login).getByRole("region", { name: "Needs Me" })).getByText("1")).toBeInTheDocument();
    });
  });

  it("counts needing Tickets in hidden States in the Board link while the browser has it on", async () => {
    localStorage.setItem("code-central.board.needs", "1");
    localStorage.setItem("code-central.board.hide", "Done,Cancelled,Failed");
    const fake = await world();
    await fake.s.ticket("acme", { title: "Broken", state: "Failed", spec: fake.ids.spec });
    await renderApp(fake, "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 4");
  });
});
