// The Round panel against FakeApi, inside the real App so each action's
// change notice refetches the Ticket the way the service's SSE does.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import type { QuestionView } from "../../api/types";

// A Ticket waiting on Round 2, whose Questions are 3 and 4 (numbers run
// across the Ticket), the first with a recommendation.
function world(questions: Partial<QuestionView>[] = []) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const posted = fake.now.toISOString();
  fake.addTicket("acme", {
    title: "Pick a colour",
    state: "Needs Answer",
    rounds: [
      {
        number: 1,
        claim: 1,
        posted,
        questions: [{ number: 1, title: "Old question", body: "Asked before", status: "answered", answer: "Yes" }],
      },
      {
        number: 2,
        claim: 1,
        posted,
        questions: [
          { number: 3, title: "Which colour?", body: "Red or **blue**, for the login button.", recommended: "Blue", status: "open" },
          { number: 4, title: "Which size?", body: "S or M", status: "open" },
          ...questions.map((q, i) => ({ number: 5 + i, title: `Extra ${i}`, body: "More", status: "open" as const, ...q })),
        ],
      },
    ],
  });
  fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 2, open: 2, questions: 2 } }] }]);
  return fake;
}

function renderPanel(fake: FakeApi, round = 2) {
  window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
  render(<App api={fake} now={() => fake.now} />);
  return screen.findByRole("region", { name: new RegExp(`Round ${round}`) });
}

const question = (panel: HTMLElement, title: string) => within(panel).getByRole("group", { name: new RegExp(title) });
const box = (q: HTMLElement) => within(q).getByRole("textbox");
const actions = (fake: FakeApi) => fake.calls.filter((c) => !["listProjects", "needsMe", "findTickets", "findSpecs", "getSpec", "getTicket"].includes(c));
const stored = async (fake: FakeApi) => (await fake.getTicket("acme", "acme-1")).rounds[1].questions;

describe("the Round panel", () => {
  it("shows every Question of the open Round, each box pre-filled with its recommendation as an unsaved draft", async () => {
    const fake = world();
    const panel = await renderPanel(fake);

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
    expect(actions(fake)).toEqual([]);
  });

  it("renders a Question's body as Markdown, keeping its title plain and its recommendation raw in the box", async () => {
    const fake = world([{ title: "Pick **one**", body: "- `left`\n- [docs](https://example.com)\n\n<b>loud</b>", recommended: "**left**" }]);
    const panel = await renderPanel(fake);

    const colour = question(panel, "Which colour?");
    expect(within(colour).getByText("blue").tagName).toBe("STRONG");

    const extra = question(panel, "Pick \\*\\*one\\*\\*");
    expect(within(extra).getByText("Pick **one**")).toBeInTheDocument();
    expect(within(extra).getAllByRole("listitem")).toHaveLength(2);
    expect(within(extra).getByText("left").tagName).toBe("CODE");
    expect(within(extra).getByRole("link", { name: "docs" })).toHaveAttribute("target", "_blank");
    expect(within(extra).getByText("<b>loud</b>")).toBeInTheDocument();
    expect(box(extra)).toHaveValue("**left**");
  });

  it("answers one Question with the text in its box, and keeps it editable until the Round is complete", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    const colour = question(panel, "Which colour?");
    await userEvent.clear(box(colour));
    await userEvent.type(box(colour), "Red");
    await userEvent.click(within(colour).getByRole("button", { name: "Answer" }));

    await waitFor(() => expect(within(question(panel, "Which colour?")).getByText("Answered")).toBeInTheDocument());
    expect(actions(fake)).toEqual(["answer"]);
    expect((await stored(fake))[0]).toMatchObject({ status: "answered", answer: "Red" });
    expect(within(panel).getByText("1 of 2 open")).toBeInTheDocument();

    // Still editable: the box holds the saved answer, and saving again changes it.
    const again = question(panel, "Which colour?");
    expect(box(again)).toHaveValue("Red");
    expect(within(again).queryByText(/Recommended answer/)).toBeNull();
    await userEvent.type(box(again), " please");
    await userEvent.click(within(again).getByRole("button", { name: "Update answer" }));
    await waitFor(async () => expect((await stored(fake))[0].answer).toBe("Red please"));
  });

  it("dismisses one Question", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.click(within(question(panel, "Which size?")).getByRole("button", { name: "Dismiss" }));

    await waitFor(() => expect(within(question(panel, "Which size?")).getByText("Dismissed")).toBeInTheDocument());
    expect(actions(fake)).toEqual(["dismiss_question"]);
    expect((await stored(fake))[1].status).toBe("dismissed");
    expect((await stored(fake))[0].status).toBe("open");
  });

  it("accepts every recommended answer in one click", async () => {
    const fake = world([{ recommended: "Yes" }]);
    const panel = await renderPanel(fake);
    await userEvent.click(within(panel).getByRole("button", { name: "Accept all recommended" }));

    await waitFor(() => expect(within(panel).getByText("1 of 3 open")).toBeInTheDocument());
    expect(actions(fake)).toEqual(["accept_recommended"]);
    expect((await stored(fake)).map((q) => [q.status, q.answer])).toEqual([
      ["answered", "Blue"],
      ["open", undefined],
      ["answered", "Yes"],
    ]);
  });

  it("offers Accept all recommended only while an open Question has a recommendation", async () => {
    const fake = world();
    fake.updateTicket("acme-1", {
      rounds: [{ number: 1, claim: 1, posted: fake.now.toISOString(), questions: [{ number: 1, title: "Which size?", body: "S or M", status: "open" }] }],
    });
    const panel = await renderPanel(fake, 1);
    expect(within(panel).queryByRole("button", { name: "Accept all recommended" })).toBeNull();
  });

  it("shows the service's refusal", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    const size = question(panel, "Which size?");
    await userEvent.type(box(size), "   ");
    await userEvent.click(within(size).getByRole("button", { name: "Answer" }));
    expect(await within(size).findByRole("alert")).toHaveTextContent("an answer needs text");
  });
});
