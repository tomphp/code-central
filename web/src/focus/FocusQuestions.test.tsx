// Question Focus Items: the real App on a WASM Core world, at ?view=focus.
// Each open Question of a Round is its own Focus Item, answered through the
// Round panel's shared form; live changes are made on the world, whose
// change notices the App receives.
import { screen, within, waitFor, act, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { acme, focus, renderApp } from "./worldTesting";

const HOUR = 3_600_000;

const dismiss = (s: Scenario, id: string, question: number) => s.human(`${s.ticketPath("acme", id)}/dismiss_question`, { question });

// round2 gives the Ticket two Rounds, as its session asked them: Round 1's
// Question (1) answered, then Round 2 with Questions 2 to 4 (numbers run
// across the Ticket): 2 and 4 open, 3 dismissed.
async function round2(s: Scenario, id: string) {
  await s.askRound("acme", id, [{ title: "Old question", body: "Asked before" }]);
  await s.answerRound("acme", id, ["Yes"]);
  await s.pickUpAnswers("acme", id);
  await s.askRound("acme", id, [
    { title: "Which colour?", body: "Red or **blue**, for the login button.", recommended: "Blue" },
    { title: "Which font?", body: "Serif or sans" },
    { title: "Which size?", body: "S or M" },
  ]);
  await dismiss(s, id, 3);
}

// world has Pick a colour waiting on Round 2 (2 open), then Login page In Review.
async function world() {
  const { w, s } = await acme();
  const review = await s.ticket("acme", { title: "Login page", state: "In Review" });
  w.advance(HOUR);
  const colour = await s.ticket("acme", { title: "Pick a colour" });
  await round2(s, colour);
  w.advance(2 * HOUR);
  return { w, s, colour, review };
}

const item = () => within(focus()).findByRole("article", { name: "Focus Item" });
const question = (title: string) => within(focus()).findByRole("heading", { name: title });
const box = () => within(focus()).getByRole("textbox");
const stored = async (w: World, id: string) => (await w.api.getTicket("acme", id)).rounds![1].questions;

describe("a Question Focus Item", () => {
  it("shows where it's from, why it waits, where it is in the Round, its title large and its body as Markdown", async () => {
    const { w, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    const it = await item();
    expect(within(it).getByText("Acme")).toBeInTheDocument();
    expect(within(it).getByText("acme")).toBeInTheDocument();
    expect(within(it).getByText(colour)).toBeInTheDocument();
    expect(within(it).getByText("Pick a colour")).toBeInTheDocument();
    expect(within(it).getByText("Focus: Questions")).toBeInTheDocument();
    expect(within(it).getByText("waiting 2h")).toBeInTheDocument();
    expect(within(it).getByText("Question 1 of 3 · Round 2")).toBeInTheDocument();
    expect(within(it).getByText("blue").tagName).toBe("STRONG");
    expect(it).not.toHaveTextContent("**");
    // One Question at a time, and not the Round panel.
    expect(within(it).queryByText("Which size?")).toBeNull();
    expect(within(it).queryByRole("button", { name: "Accept all recommended" })).toBeNull();
  });

  it("counts each open Question in the remaining count", async () => {
    const { w } = await world();
    renderApp(w);
    await question("Which colour?");
    expect(within(focus()).getByText("3 left")).toBeInTheDocument();
  });

  it("prefills the answer box with the recommendation as an unsaved draft", async () => {
    const { w } = await world();
    renderApp(w);
    await question("Which colour?");
    expect(box()).toHaveValue("Blue");
    expect(within(focus()).getByText(/Recommended answer, a draft: nothing is saved until you answer/)).toBeInTheDocument();
    expect(w.actions()).toEqual([]);
  });

  it("labels a Stage Ticket's Question with its Requirement and Stage", async () => {
    const { w, s } = await acme();
    const spec = await s.spec("acme", { title: "Dark mode" }); // acme-1
    const stage = await s.ticket("acme", { title: "Grilling: Dark mode", spec, labels: ["stage:grilling"] }); // acme-2
    await round2(s, stage);
    renderApp(w);
    expect(await question("Which colour?")).toBeInTheDocument();
    expect(within(await item()).getByText("Requirement: Dark mode · grilling")).toBeInTheDocument();
    expect(within(await item()).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", `?view=inbox&requirement=${spec}&ticket=${stage}`);
  });
});

describe("answering in Focus Mode", () => {
  it("sends Answer for that Question, then shows the Round's next open Question", async () => {
    const { w, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));

    expect(await question("Which size?")).toBeInTheDocument();
    expect(within(focus()).getByText("Question 3 of 3 · Round 2")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(box()).toHaveValue("");
    expect(w.actions()).toEqual(["answer"]);
    expect((await stored(w, colour))[0]).toMatchObject({ status: "answered", answer: "Blue" });
  });

  it("sends Dismiss for that Question", async () => {
    const { w, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));

    expect(await question("Which size?")).toBeInTheDocument();
    expect(w.actions()).toEqual(["dismiss_question"]);
    expect((await stored(w, colour))[0].status).toBe("dismissed");
  });

  it("sends Answer on Cmd+Enter or Ctrl+Enter in the answer box", async () => {
    const { w, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    await userEvent.clear(box());
    await userEvent.type(box(), "Green");
    fireEvent.keyDown(box(), { key: "Enter", metaKey: true });

    await question("Which size?");
    expect((await stored(w, colour))[0]).toMatchObject({ status: "answered", answer: "Green" });

    await userEvent.type(box(), "M");
    fireEvent.keyDown(box(), { key: "Enter", ctrlKey: true });
    expect(await within(focus()).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
    expect((await stored(w, colour))[2]).toMatchObject({ status: "answered", answer: "M" });
    expect(w.actions()).toEqual(["answer", "answer"]);
  });

  it("sends nothing on a plain Enter, nor on Cmd+Enter in an empty box", async () => {
    const { w, s, colour } = await world();
    await dismiss(s, colour, 2);
    renderApp(w);
    await question("Which size?");
    fireEvent.keyDown(box(), { key: "Enter", metaKey: true });
    await userEvent.type(box(), "M{Enter}");
    expect(box()).toHaveValue("M\n");
    expect(w.actions()).toEqual([]);
  });

  it("leaves the Inbox's Round panel without Cmd+Enter", async () => {
    const { w, colour } = await world();
    renderApp(w, `/?view=inbox&ticket=${colour}`);
    const panel = await screen.findByRole("region", { name: /Round 2/ });
    const group = within(panel).getByRole("group", { name: /Which colour\?/ });
    fireEvent.keyDown(within(group).getByRole("textbox"), { key: "Enter", metaKey: true });
    fireEvent.keyDown(within(group).getByRole("textbox"), { key: "Enter", ctrlKey: true });
    expect(w.actions()).toEqual([]);
  });

  it("goes on, after the Round's last Question, to the Ticket as an Answered Focus Item in its Needs Me place", async () => {
    const { w } = await world();
    renderApp(w);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    await question("Which size?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));

    const it = await item();
    expect(await within(it).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
    expect(within(it).getByText("Pick a colour")).toBeInTheDocument();
    expect(within(it).getByText("Focus: Answered")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });
});

describe("a Round's Questions in the queue", () => {
  it("come at the row's Needs Me place, after what's before it", async () => {
    // Core lists Needs Answer longest-waiting first, and no reason comes
    // before it, so what's before the Round is an older Round.
    const { w, s } = await acme();
    const older = await s.ticket("acme", { title: "Older question" });
    await s.askRound("acme", older, [{ title: "Which tool?", body: "A or B" }]);
    w.advance(HOUR);
    const colour = await s.ticket("acme", { title: "Pick a colour" });
    await round2(s, colour);
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    renderApp(w);
    expect(await within(focus()).findByRole("heading", { name: "Which tool?" })).toBeInTheDocument();
    expect(within(focus()).getByText("4 left")).toBeInTheDocument();

    await act(() => s.answerRound("acme", older));
    expect(await question("Which colour?")).toBeInTheDocument();
  });

  it("finishes the Round first, even when another Focus Item arrives meanwhile", async () => {
    const { w, s, review } = await world();
    await s.ticket("acme", { title: "Quiet session", state: "In Progress" });
    renderApp(w);
    await question("Which colour?");
    // Silent after 30 minutes; a change to another Ticket makes the App look again.
    w.advance(31 * 60_000);
    await act(() => s.human(`${s.projectPath("acme")}/add_labels`, { target: review, labels: ["urgent"] }));
    await waitFor(() => expect(within(focus()).getByText("4 left")).toBeInTheDocument());
    expect(within(focus()).getByRole("heading", { name: "Which colour?" })).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    expect(await question("Which size?")).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    expect(await within(focus()).findByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
  });

  it("moves on to the Round's next open Question when the one on screen is closed elsewhere", async () => {
    const { w, s, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    await act(() => dismiss(s, colour, 2));
    expect(await question("Which size?")).toBeInTheDocument();
  });

  it("moves on when its Ticket leaves Needs Answer", async () => {
    const { w, s, colour } = await world();
    renderApp(w);
    await question("Which colour?");
    await act(() => s.human(`${s.ticketPath("acme", colour)}/cancel`, {}));
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });

  it("leaves a Round out of a Project filter that doesn't cover it", async () => {
    const { w, s } = await world();
    await s.project("beta", "Beta");
    await s.ticket("beta", { title: "Beta review", state: "In Review" });
    renderApp(w, "/?view=focus&project=beta");
    expect(await within(focus()).findByRole("heading", { name: "Beta review" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });
});
