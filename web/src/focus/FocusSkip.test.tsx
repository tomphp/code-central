// Skip in Focus Mode: the real App on a WASM Core world, at ?view=focus. A
// skipped Focus Item goes to the back of the queue, a skipped Question
// lets its Round carry on, and once only skipped items are left Focus Mode
// offers to go through them again. Skips live in the tab's memory only.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { acme, allClearOn, focus, renderApp, settle } from "./worldTesting";

// waiting is acme with All clear on and, in Needs Me's order, a Ticket
// waiting on Round 1 (Questions 1 and 2 open), a Failed one and an In
// Review one. round: false and tickets: false leave those out.
async function waiting({ round = true, tickets = true } = {}) {
  const { w, s } = await acme();
  await allClearOn(w);
  if (round) {
    const id = await s.ticket("acme", { title: "Pick a colour" });
    await s.askRound("acme", id, [
      { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
      { title: "Which size?", body: "S or M" },
    ]);
  }
  let failed = "";
  let review = "";
  if (tickets) {
    failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    review = await s.ticket("acme", { title: "Login page", state: "In Review" });
  }
  return { w, s, failed, review };
}

// waitingTickets is waiting without its Round, so every Focus Item is a
// Ticket's.
const waitingTickets = () => waiting({ round: false });

// quiet has an agent claim a Ticket and go silent for long enough to be a
// Silent Claim, which a write after it makes known.
async function quiet(w: World, s: Scenario) {
  await s.ticket("acme", { title: "Quiet session", state: "In Progress" });
  w.advance(31 * 60_000);
  await s.ticket("acme", { title: "Filler" }); // sends the change notice
}

const heading = (name: string) => within(focus()).getByRole("heading", { name });
// skip clicks Skip and settles what it sets off.
const skip = async (w: World) => {
  await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
  await settle(w);
};

describe("Skip", () => {
  it("sits next to Answer, Dismiss and Prototype this on a Question Focus Item", async () => {
    const { w } = await waiting();
    await renderApp(w);
    expect(heading("Which colour?")).toBeInTheDocument();
    const buttons = within(focus()).getByRole("button", { name: "Answer" }).parentElement!;
    expect(within(buttons).getAllByRole("button").map((b) => b.textContent)).toEqual(["Answer", "Dismiss", "Prototype this", "Skip"]);
  });

  it("sits with a Ticket Focus Item's panel buttons", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    expect(heading("Flaky build")).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Send back" })).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Skip" })).toBeInTheDocument();
  });

  it("sends the item to the back of the queue and shows the next one, still counting it", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    expect(heading("Flaky build")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    await skip(w);

    expect(heading("Login page")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(w.actions()).toEqual([]);
  });

  it("goes on to the Round's next open Question, and only the skipped one goes to the back of the whole queue", async () => {
    const { w } = await waiting();
    await renderApp(w);
    await skip(w);
    expect(heading("Which size?")).toBeInTheDocument();
    expect(within(focus()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    await settle(w);
    expect(heading("Flaky build")).toBeInTheDocument();
  });

  it("keeps a skipped item behind one that arrives later", async () => {
    const { w, s } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    await quiet(w, s);
    await skip(w);

    expect(heading("Quiet session")).toBeInTheDocument();
  });

  it("has no single-key shortcut", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    await userEvent.keyboard("s");
    await userEvent.keyboard("k");
    expect(heading("Flaky build")).toBeInTheDocument();
  });
});

describe("only skipped items left", () => {
  it("asks \"You skipped N, go through them again?\" instead of All clear, still counting them", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    await skip(w);

    expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();
    expect(within(focus()).queryByRole("article", { name: "Focus Item" })).toBeNull();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("counts a skipped Round's open Questions one each", async () => {
    const { w } = await waiting({ tickets: false });
    await renderApp(w);
    await skip(w);
    await skip(w);

    expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
  });

  it("goes through them again from the front, in Needs Me order", async () => {
    const { w } = await waiting();
    await renderApp(w);
    await skip(w);
    await skip(w);
    await skip(w);
    await skip(w);
    expect(within(focus()).getByText("You skipped 4, go through them again?")).toBeInTheDocument();
    await userEvent.click(within(focus()).getByRole("button", { name: "Go through them again" }));
    await settle(w);

    expect(heading("Which colour?")).toBeInTheDocument();
    await skip(w);
    expect(heading("Which size?")).toBeInTheDocument();
  });

  it("shows a new Focus Item on its own when it arrives", async () => {
    const { w, s } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    await skip(w);
    expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
    await quiet(w, s);

    expect(heading("Quiet session")).toBeInTheDocument();
  });

  it("is All clear once the skipped items are dealt with elsewhere", async () => {
    const { w, s, failed, review } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    await skip(w);
    expect(within(focus()).getByText("You skipped 2, go through them again?")).toBeInTheDocument();
    await s.human(`${s.ticketPath("acme", failed)}/cancel`, {});
    await s.human(`${s.ticketPath("acme", review)}/accept`, {});

    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
  });
});

describe("skips", () => {
  it("live only in this tab's memory: never sent, never in the URL or local storage", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    expect(heading("Login page")).toBeInTheDocument();

    expect(window.location.search).toBe("?view=focus");
    expect(localStorage.length).toBe(0);
    expect(w.actions()).toEqual([]);
  });

  it("are forgotten on leaving Focus Mode", async () => {
    const { w } = await waitingTickets();
    await renderApp(w);
    await skip(w);
    expect(heading("Login page")).toBeInTheDocument();
    await userEvent.click(within(focus()).getByRole("button", { name: "Exit" }));
    await settle(w);
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));
    await settle(w);

    expect(heading("Flaky build")).toBeInTheDocument();
  });

  it("are forgotten on reload", async () => {
    const { w } = await waitingTickets();
    const { unmount } = await renderApp(w);
    await skip(w);
    expect(heading("Login page")).toBeInTheDocument();
    unmount();
    await renderApp(w);

    expect(heading("Flaky build")).toBeInTheDocument();
  });
});
