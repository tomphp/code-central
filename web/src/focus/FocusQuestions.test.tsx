// Question Focus Items: the real App against FakeApi, at ?view=focus. Each
// open Question of a Round is its own Focus Item, answered through the
// Round panel's shared form; live changes are simulated by reseeding Needs
// Me and emitting a notice.
import { render, screen, within, waitFor, act, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { RoundView } from "../api/types";

const hourAgo = (fake: FakeApi, h: number) => new Date(fake.now.getTime() - h * 3_600_000).toISOString();

// round2 is Round 2 of acme-1, whose Questions are 3 to 5 (numbers run
// across the Ticket): 3 and 5 open, 4 already dismissed.
function round2(posted: string): RoundView[] {
  return [
    { number: 1, claim: 1, posted, questions: [{ number: 1, title: "Old question", body: "Asked before", status: "answered", answer: "Yes" }] },
    {
      number: 2,
      claim: 1,
      posted,
      questions: [
        { number: 3, title: "Which colour?", body: "Red or **blue**, for the login button.", recommended: "Blue", status: "open" },
        { number: 4, title: "Which font?", body: "Serif or sans", status: "dismissed" },
        { number: 5, title: "Which size?", body: "S or M", status: "open" },
      ],
    },
  ];
}

// world has acme-1 waiting on Round 2 (2 open), then acme-2 In Review.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer", rounds: round2(fake.now.toISOString()) }); // acme-1
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-2
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-1", since: hourAgo(fake, 2), round: { number: 2, open: 2, questions: 3 } }] },
    { reason: "in_review", tickets: [{ id: "acme-2", since: hourAgo(fake, 3), summary: "Added the login page" }] },
  ]);
  return fake;
}

function renderApp(fake: FakeApi, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const focus = () => screen.getByRole("main", { name: "Focus Mode" });
const item = () => within(focus()).findByRole("article", { name: "Focus Item" });
const question = (title: string) => within(focus()).findByRole("heading", { name: title });
const box = () => within(focus()).getByRole("textbox");
const stored = async (fake: FakeApi, id = "acme-1") => (await fake.getTicket("acme", id)).rounds[1].questions;

// change reseeds Needs Me and sends the notice the service would.
function change(fake: FakeApi, groups: Parameters<FakeApi["setNeedsMe"]>[0], id = "acme-1") {
  fake.setNeedsMe(groups);
  act(() => fake.emit({ type: "ticket.changed", project: id.split("-")[0], id }));
}

describe("a Question Focus Item", () => {
  it("shows where it's from, why it waits, where it is in the Round, its title large and its body as Markdown", async () => {
    renderApp(world());
    await question("Which colour?");
    const it = await item();
    expect(within(it).getByText("Acme")).toBeInTheDocument();
    expect(within(it).getByText("acme")).toBeInTheDocument();
    expect(within(it).getByText("acme-1")).toBeInTheDocument();
    expect(within(it).getByText("Pick a colour")).toBeInTheDocument();
    expect(within(it).getByText("Questions")).toBeInTheDocument();
    expect(within(it).getByText("waiting 2h")).toBeInTheDocument();
    expect(within(it).getByText("Question 1 of 3 · Round 2")).toBeInTheDocument();
    expect(within(it).getByText("blue").tagName).toBe("STRONG");
    expect(it).not.toHaveTextContent("**");
    // One Question at a time, and not the Round panel.
    expect(within(it).queryByText("Which size?")).toBeNull();
    expect(within(it).queryByRole("button", { name: "Accept all recommended" })).toBeNull();
  });

  it("counts each open Question in the remaining count", async () => {
    renderApp(world());
    await question("Which colour?");
    expect(within(focus()).getByText("3 left")).toBeInTheDocument();
  });

  it("prefills the answer box with the recommendation as an unsaved draft", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    expect(box()).toHaveValue("Blue");
    expect(within(focus()).getByText(/Recommended answer, a draft: nothing is saved until you answer/)).toBeInTheDocument();
    expect(fake.actions()).toEqual([]);
  });

  it("labels a Stage Ticket's Question with its Requirement and Stage", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const spec = fake.addSpec("acme", { title: "Dark mode" }); // acme-1
    const stage = fake.addTicket("acme", {
      title: "Grilling: Dark mode",
      state: "Needs Answer",
      spec: { id: spec.id, title: spec.title },
      labels: ["stage:grilling"],
      rounds: round2(fake.now.toISOString()),
    }); // acme-2
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: stage.id, round: { number: 2, open: 2, questions: 3 } }] }]);
    renderApp(fake);
    expect(await question("Which colour?")).toBeInTheDocument();
    expect(within(await item()).getByText("Requirement: Dark mode · grilling")).toBeInTheDocument();
    expect(within(await item()).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", `?view=inbox&requirement=${spec.id}&ticket=${stage.id}`);
  });
});

describe("answering in Focus Mode", () => {
  it("sends Answer for that Question, then shows the Round's next open Question", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));

    expect(await question("Which size?")).toBeInTheDocument();
    expect(within(focus()).getByText("Question 3 of 3 · Round 2")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(box()).toHaveValue("");
    expect(fake.actions()).toEqual(["answer"]);
    expect((await stored(fake))[0]).toMatchObject({ status: "answered", answer: "Blue" });
  });

  it("sends Dismiss for that Question", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));

    expect(await question("Which size?")).toBeInTheDocument();
    expect(fake.actions()).toEqual(["dismiss_question"]);
    expect((await stored(fake))[0].status).toBe("dismissed");
  });

  it("sends Answer on Cmd+Enter or Ctrl+Enter in the answer box", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    await userEvent.clear(box());
    await userEvent.type(box(), "Green");
    fireEvent.keyDown(box(), { key: "Enter", metaKey: true });

    await question("Which size?");
    expect((await stored(fake))[0]).toMatchObject({ status: "answered", answer: "Green" });

    await userEvent.type(box(), "M");
    fireEvent.keyDown(box(), { key: "Enter", ctrlKey: true });
    expect(await within(focus()).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
    expect((await stored(fake))[2]).toMatchObject({ status: "answered", answer: "M" });
    expect(fake.actions()).toEqual(["answer", "answer"]);
  });

  it("sends nothing on a plain Enter, nor on Cmd+Enter in an empty box", async () => {
    const fake = world();
    fake.updateTicket("acme-1", { rounds: round2(fake.now.toISOString()).map((r) => ({ ...r, questions: r.questions.filter((q) => q.number !== 3) })) });
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 2, open: 1, questions: 2 } }] }]);
    renderApp(fake);
    await question("Which size?");
    fireEvent.keyDown(box(), { key: "Enter", metaKey: true });
    await userEvent.type(box(), "M{Enter}");
    expect(box()).toHaveValue("M\n");
    expect(fake.actions()).toEqual([]);
  });

  it("leaves the Inbox's Round panel without Cmd+Enter", async () => {
    const fake = world();
    renderApp(fake, "/?view=inbox&ticket=acme-1");
    const panel = await screen.findByRole("region", { name: /Round 2/ });
    const colour = within(panel).getByRole("group", { name: /Which colour\?/ });
    fireEvent.keyDown(within(colour).getByRole("textbox"), { key: "Enter", metaKey: true });
    fireEvent.keyDown(within(colour).getByRole("textbox"), { key: "Enter", ctrlKey: true });
    expect(fake.actions()).toEqual([]);
  });

  it("goes on, after the Round's last Question, to the Ticket as an Answered Focus Item in its Needs Me place", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    await question("Which size?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));

    const it = await item();
    expect(await within(it).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
    expect(within(it).getByText("Pick a colour")).toBeInTheDocument();
    expect(within(it).getByText("Answered")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });
});

describe("a Round's Questions in the queue", () => {
  it("come at the row's Needs Me place, after what's before it", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-3
    fake.setNeedsMe([{ reason: "failed", tickets: [{ id: "acme-3" }] }, ...fake.needsMeGroups]);
    renderApp(fake);
    expect(await within(focus()).findByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(within(focus()).getByText("4 left")).toBeInTheDocument();

    change(fake, fake.needsMeGroups.filter((g) => g.reason !== "failed"), "acme-3");
    expect(await question("Which colour?")).toBeInTheDocument();
  });

  it("finishes the Round first, even when a higher-priority Focus Item arrives meanwhile", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-3
    change(fake, [{ reason: "silent_claim", tickets: [{ id: t.id }] }, ...fake.needsMeGroups], t.id);
    await waitFor(() => expect(within(focus()).getByText("4 left")).toBeInTheDocument());
    expect(within(focus()).getByRole("heading", { name: "Which colour?" })).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    expect(await question("Which size?")).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    expect(await within(focus()).findByRole("heading", { name: "Quiet session" })).toBeInTheDocument();
  });

  it("finishes the Ticket's latest Round first when its Needs Me row names no Round", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-1" }] }, ...fake.needsMeGroups.slice(1)]);
    renderApp(fake);
    await question("Which colour?");
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-3
    change(fake, [{ reason: "silent_claim", tickets: [{ id: t.id }] }, ...fake.needsMeGroups], t.id);
    await waitFor(() => expect(within(focus()).getByText("3 left")).toBeInTheDocument());

    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    expect(await question("Which size?")).toBeInTheDocument();
  });

  it("moves on to the Round's next open Question when the one on screen is closed elsewhere", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    await act(() => fake.act("acme", "acme-1", "dismiss_question", { question: 3 }));
    expect(await question("Which size?")).toBeInTheDocument();
  });

  it("moves on when its Ticket leaves Needs Answer", async () => {
    const fake = world();
    renderApp(fake);
    await question("Which colour?");
    change(fake, fake.needsMeGroups.filter((g) => g.reason !== "needs_answer"));
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });

  it("leaves a Round out of a Project filter that doesn't cover it", async () => {
    const fake = world();
    fake.addProject("beta", "Beta");
    fake.addTicket("beta", { title: "Beta review", state: "In Review" }); // beta-1
    fake.setNeedsMe([...fake.needsMeGroups, { reason: "accepted_not_landed", tickets: [{ id: "beta-1" }] }]);
    renderApp(fake, "/?view=focus&project=beta");
    expect(await within(focus()).findByRole("heading", { name: "Beta review" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });
});
