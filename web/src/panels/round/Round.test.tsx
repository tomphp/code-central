// The Round panel on the WASM Core, inside the real App so each action's
// change notice refetches the Ticket the way the service's SSE does.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../../api/core/core";
import type { QuestionSeed } from "../../api/scenario";
import { renderApp, scenario, settle } from "../../worldTesting";

const COLOUR: QuestionSeed = { title: "Which colour?", body: "Red or **blue**, for the login button.", recommended: "Blue" };
const SIZE: QuestionSeed = { title: "Which size?", body: "S or M" };

// world has a Ticket waiting on Round 2, whose Questions are 3 and 4 (numbers
// run across the Ticket), the first with a recommendation, then any extra
// ones; Round 1 was answered.
async function world(extra: Partial<QuestionSeed>[] = [], questions: QuestionSeed[] = [COLOUR, SIZE]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Pick a colour" });
  await s.askRound("acme", id, [{ title: "Old question", body: "Asked before" }]);
  await s.answerRound("acme", id, ["Yes"]);
  await s.pickUpAnswers("acme", id);
  await s.askRound("acme", id, [...questions, ...extra.map((q, i) => ({ title: `Extra ${i}`, body: "More", ...q }))]);
  w.calls.length = 0;
  return { w, id };
}

type Waiting = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, id }: Waiting, round = 2) {
  await renderApp(w, `/?view=inbox&ticket=${id}`);
  return screen.getByRole("region", { name: new RegExp(`Round ${round}`) });
}

const question = (panel: HTMLElement, title: string) => within(panel).getByRole("group", { name: new RegExp(title) });
const box = (q: HTMLElement) => within(q).getByRole("textbox");
const stored = async ({ w, id }: Waiting) => (await w.api.getTicket("acme", id)).rounds![1].questions;

describe("the Round panel", () => {
  it("shows every Question of the open Round, each box pre-filled with its recommendation as an unsaved draft", async () => {
    const x = await world();
    const panel = await renderPanel(x);

    expect(within(panel).queryByText("Old question")).toBeNull();
    const colour = question(panel, "Which colour?");
    expect(colour).toHaveTextContent("Red or blue, for the login button.");
    expect(colour).not.toHaveTextContent("**");
    expect(box(colour)).toHaveValue("Blue");
    expect(within(colour).getByText(/Recommended answer, a draft: nothing is saved until you answer/)).toBeInTheDocument();

    const size = question(panel, "Which size?");
    expect(size).toHaveTextContent("S or M");
    expect(box(size)).toHaveValue("");
    expect(within(size).queryByText(/Recommended/)).toBeNull();
    expect(within(size).getByRole("button", { name: "Answer" })).toBeDisabled();

    expect(within(panel).getByText("2 of 2 open")).toBeInTheDocument();
    expect(x.w.actions()).toEqual([]);
  });

  it("renders a Question's body as Markdown, keeping its title plain and its recommendation raw in the box", async () => {
    const x = await world([{ title: "Pick _one_", body: "- left\n- [right](https://example.com)\n\n<b>loud</b>", recommended: "**left**" }]);
    const panel = await renderPanel(x);

    const colour = question(panel, "Which colour?");
    expect(within(colour).getByText("blue").tagName).toBe("STRONG");

    const extra = question(panel, "Pick _one_");
    expect(within(extra).getAllByRole("listitem")).toHaveLength(2);
    expect(within(extra).getByRole("link", { name: "right" })).toBeInTheDocument();
    expect(within(extra).getByText("<b>loud</b>")).toBeInTheDocument();
    expect(box(extra)).toHaveValue("**left**");
  });

  it("answers one Question with the text in its box, and keeps it editable until the Round is complete", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    const colour = question(panel, "Which colour?");
    await userEvent.clear(box(colour));
    await userEvent.type(box(colour), "Red");
    await userEvent.click(within(colour).getByRole("button", { name: "Answer" }));

    await settle(x.w);
    expect(within(question(panel, "Which colour?")).getByText("Answered")).toBeInTheDocument();
    expect(x.w.actions()).toEqual(["answer"]);
    expect((await stored(x))[0]).toMatchObject({ status: "answered", answer: "Red" });
    expect(within(panel).getByText("1 of 2 open")).toBeInTheDocument();

    // Still editable: the box holds the saved answer, and saving again changes it.
    const again = question(panel, "Which colour?");
    expect(box(again)).toHaveValue("Red");
    expect(within(again).queryByText(/Recommended answer/)).toBeNull();
    await userEvent.type(box(again), " please");
    await userEvent.click(within(again).getByRole("button", { name: "Update answer" }));
    await settle(x.w);
    expect((await stored(x))[0].answer).toBe("Red please");
  });

  it("dismisses one Question", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.click(within(question(panel, "Which size?")).getByRole("button", { name: "Dismiss" }));

    await settle(x.w);
    expect(within(question(panel, "Which size?")).getByText("Dismissed")).toBeInTheDocument();
    expect(x.w.actions()).toEqual(["dismiss_question"]);
    expect((await stored(x))[1].status).toBe("dismissed");
    expect((await stored(x))[0].status).toBe("open");
  });

  it("accepts every recommended answer in one click", async () => {
    const x = await world([{ recommended: "Yes" }]);
    const panel = await renderPanel(x);
    await userEvent.click(within(panel).getByRole("button", { name: "Accept all recommended" }));

    await settle(x.w);
    expect(within(panel).getByText("1 of 3 open")).toBeInTheDocument();
    expect(x.w.actions()).toEqual(["accept_recommended"]);
    expect((await stored(x)).map((q) => [q.status, q.answer])).toEqual([
      ["answered", "Blue"],
      ["open", undefined],
      ["answered", "Yes"],
    ]);
  });

  it("offers Accept all recommended only while an open Question has a recommendation", async () => {
    const panel = await renderPanel(await world([], [SIZE]));
    expect(within(panel).queryByRole("button", { name: "Accept all recommended" })).toBeNull();
  });

  it("fills a Question's box with \"Prototype this: \" for the human to add a note to, saving nothing", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    const colour = question(panel, "Which colour?");
    await userEvent.click(within(colour).getByRole("button", { name: "Prototype this" }));

    expect(box(colour)).toHaveValue("Prototype this: ");
    expect(box(colour)).toHaveFocus();
    expect(within(colour).queryByText(/Recommended answer/)).toBeNull();
    expect(box(question(panel, "Which size?"))).toHaveValue("");
    expect(x.w.actions()).toEqual([]);

    await userEvent.type(box(colour), "try both on the login page");
    await userEvent.click(within(colour).getByRole("button", { name: "Answer" }));
    await settle(x.w);
    expect((await stored(x))[0].answer).toBe("Prototype this: try both on the login page");
  });

  it("shows the service's refusal", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    const size = question(panel, "Which size?");
    await userEvent.type(box(size), "   ");
    await userEvent.click(within(size).getByRole("button", { name: "Answer" }));
    await settle(x.w);
    expect(within(size).getByRole("alert")).toHaveTextContent("an answer needs text");
  });
});
