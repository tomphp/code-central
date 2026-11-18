// Board view tests: a card's Ticket popup and Focus popup.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WORK, type QuestionSeed } from "../api/scenario";
import { ApiError } from "../api/types";
import { failCI } from "../ciTesting";
import { world, renderApp, settled, click, travel, press, board, heads, card, loaded, noPopup } from "./Board.testutil";

beforeEach(() => localStorage.clear());

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
    await noPopup();
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
    await noPopup();
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
    await noPopup();
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
    await noPopup();
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

  it("opens from a card that needs the human, with its Focus Item and no Skip or N left, and its title linking to the Ticket", async () => {
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
    expect(within(item()).getByRole("link", { name: "Pick a colour" })).toHaveAttribute(
      "href",
      `?view=inbox&project=acme&ticket=${fake.pick}`,
    );
  });

  it("leaves for the Ticket in the Inbox from its title, closing the popup, and Back returns to it", async () => {
    const fake = await asking();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(within(card("Pick a colour")).getByText("Pick a colour"));
    await click(within(item()).getByRole("link", { name: "Pick a colour" }));
    await settled();
    expect(screen.queryByRole("dialog", { name: "Focus popup" })).toBeNull();
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${fake.pick}`);
    expect(screen.getByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
    window.history.back();
    await settled();
    expect(window.location.search).toBe(`?view=board&project=acme&popup=${fake.pick}`);
    expect(popup()).toBeInTheDocument();
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
    await noPopup();
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
    await noPopup();
  });

  it("closes quietly when the agent deals with its Ticket", async () => {
    const fake = await asking();
    await renderApp(fake, `/?view=board&project=acme&popup=${fake.pick}`);
    question("Which colour?");
    const length = window.history.length;
    await fake.w.api.act("acme", fake.pick, "move", { to_state: "In Progress" });
    await settled();
    await noPopup();
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
    // Its title is plain text: no one Ticket to go to.
    expect(within(within(ciItem).getByRole("heading", { level: 2 })).queryByRole("link")).toBeNull();
    const panel = within(ciItem).getByRole("region", { name: "CI failed" });
    await click(within(within(panel).getByRole("group", { name: "Failure options" })).getByRole("button", { name: "Land anyway" }));
    for (const id of [basket, checkout]) expect((await fake.w.api.getTicket("acme", id)).land_anyway).toBe(true);
    await noPopup();
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
    await noPopup();
    expect(window.location.search).toBe("?view=board&project=acme");
    expect(card("Pick a colour")).toHaveFocus();
  });
});
