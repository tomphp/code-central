// Skip in Focus Mode: the real App against FakeApi, at ?view=focus. A
// skipped Focus Item goes to the back of the queue, a skipped Question
// lets its Round carry on, and once only skipped items are left Focus Mode
// offers to go through them again. Skips live in the tab's memory only.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { change, focus, renderApp, waiting, waitingTickets } from "./focusTesting";

const heading = (name: string) => within(focus()).findByRole("heading", { name });
// skip waits for Skip, which shows once the Focus Item has loaded.
const skip = async () => userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

describe("Skip", () => {
  it("sits next to Answer, Dismiss and Prototype this on a Question Focus Item", async () => {
    renderApp(waiting());
    await heading("Which colour?");
    const buttons = within(focus()).getByRole("button", { name: "Answer" }).parentElement!;
    expect(within(buttons).getAllByRole("button").map((b) => b.textContent)).toEqual(["Answer", "Dismiss", "Prototype this", "Skip"]);
  });

  it("sits with a Ticket Focus Item's panel buttons", async () => {
    renderApp(waitingTickets());
    await heading("Flaky build");
    expect(await within(focus()).findByRole("button", { name: "Send back" })).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Skip" })).toBeInTheDocument();
  });

  it("sends the item to the back of the queue and shows the next one, still counting it", async () => {
    const fake = waitingTickets();
    renderApp(fake);
    await heading("Flaky build");
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    await skip();

    expect(await heading("Login page")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(fake.actions()).toEqual([]);
  });

  it("goes on to the Round's next open Question, and only the skipped one goes to the back of the whole queue", async () => {
    renderApp(waiting());
    await heading("Which colour?");
    await skip();
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(within(focus()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    expect(await heading("Flaky build")).toBeInTheDocument();
  });

  it("keeps a skipped item behind one that arrives later", async () => {
    const fake = waitingTickets();
    renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-4
    change(fake, [...fake.needsMeGroups, { reason: "silent_claim", tickets: [{ id: t.id }] }], t.id);
    await skip();

    expect(await heading("Quiet session")).toBeInTheDocument();
  });

  it("has no single-key shortcut", async () => {
    renderApp(waitingTickets());
    await heading("Flaky build");
    await userEvent.keyboard("s");
    await userEvent.keyboard("k");
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });
});

describe("only skipped items left", () => {
  it("asks \"You skipped N, go through them again?\" instead of All clear, still counting them", async () => {
    renderApp(waitingTickets());
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await skip();

    expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();
    expect(within(focus()).queryByRole("article", { name: "Focus Item" })).toBeNull();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("counts a skipped Round's open Questions one each", async () => {
    const fake = waiting();
    fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason === "needs_answer"));
    renderApp(fake);
    await heading("Which colour?");
    await skip();
    await heading("Which size?");
    await skip();

    expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
  });

  it("goes through them again from the front, in Needs Me order", async () => {
    renderApp(waiting());
    await heading("Which colour?");
    await skip();
    await heading("Which size?");
    await skip();
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await skip();
    await within(focus()).findByText("You skipped 4, go through them again?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Go through them again" }));

    expect(await heading("Which colour?")).toBeInTheDocument();
    await skip();
    expect(await heading("Which size?")).toBeInTheDocument();
  });

  it("shows a new Focus Item on its own when it arrives", async () => {
    const fake = waitingTickets();
    renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await skip();
    await within(focus()).findByText("You skipped 2, go through them again?");
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-4
    change(fake, [...fake.needsMeGroups, { reason: "silent_claim", tickets: [{ id: t.id }] }], t.id);

    expect(await heading("Quiet session")).toBeInTheDocument();
  });

  it("is All clear once the skipped items are dealt with elsewhere", async () => {
    const fake = waitingTickets();
    renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await skip();
    await within(focus()).findByText("You skipped 2, go through them again?");
    change(fake, [], "acme-2");

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
  });
});

describe("skips", () => {
  it("live only in this tab's memory: never sent, never in the URL or local storage", async () => {
    const fake = waitingTickets();
    renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");

    expect(window.location.search).toBe("?view=focus");
    expect(localStorage.length).toBe(0);
    expect(fake.actions()).toEqual([]);
  });

  it("are forgotten on leaving Focus Mode", async () => {
    renderApp(waitingTickets());
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await userEvent.click(within(focus()).getByRole("button", { name: "Exit" }));
    await userEvent.click(within(await screen.findByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));

    expect(await heading("Flaky build")).toBeInTheDocument();
  });

  it("are forgotten on reload", async () => {
    const fake = waitingTickets();
    const { unmount } = renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    unmount();
    renderApp(fake);

    expect(await heading("Flaky build")).toBeInTheDocument();
  });
});
