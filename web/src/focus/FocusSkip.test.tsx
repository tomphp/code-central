// Skip in Focus Mode: the real App against FakeApi, at ?view=focus. A
// skipped Focus Item goes to the back of the queue, a skipped Question
// lets its Round carry on, and once only skipped items are left Focus Mode
// offers to go through them again. Skips live in the tab's memory only.
import { render, screen, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { RoundView } from "../api/types";

// world has acme-1 waiting on Round 1 (Questions 1 and 2 open), then
// acme-2 Failed and acme-3 In Review, in that Needs Me order.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const rounds: RoundView[] = [
    {
      number: 1,
      claim: 1,
      posted: fake.now.toISOString(),
      questions: [
        { number: 1, title: "Which colour?", body: "Red or blue", recommended: "Blue", status: "open" },
        { number: 2, title: "Which size?", body: "S or M", status: "open" },
      ],
    },
  ];
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer", rounds }); // acme-1
  fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-2
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-3
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 1, open: 2, questions: 2 } }] },
    { reason: "failed", tickets: [{ id: "acme-2" }] },
    { reason: "in_review", tickets: [{ id: "acme-3", summary: "Added the login page" }] },
  ]);
  return fake;
}

// tickets is world without its Round, so every Focus Item is a Ticket's.
function tickets() {
  const fake = world();
  fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason !== "needs_answer"));
  return fake;
}

function renderApp(fake: FakeApi, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const focus = () => screen.getByRole("main", { name: "Focus Mode" });
const heading = (name: string) => within(focus()).findByRole("heading", { name });
// skip waits for Skip, which shows once the Focus Item has loaded.
const skip = async () => userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));
// change reseeds Needs Me and sends the notice the service would.
function change(fake: FakeApi, groups: Parameters<FakeApi["setNeedsMe"]>[0], id: string) {
  fake.setNeedsMe(groups);
  act(() => fake.emit({ type: "ticket.changed", project: id.split("-")[0], id }));
}

const actions = (fake: FakeApi) => fake.calls.filter((c) => !["listProjects", "needsMe", "findTickets", "findSpecs", "getSpec", "getTicket"].includes(c));

describe("Skip", () => {
  it("sits next to Answer and Dismiss on a Question Focus Item", async () => {
    renderApp(world());
    await heading("Which colour?");
    const buttons = within(focus()).getByRole("button", { name: "Answer" }).parentElement!;
    expect(within(buttons).getAllByRole("button").map((b) => b.textContent)).toEqual(["Answer", "Dismiss", "Skip"]);
  });

  it("sits with a Ticket Focus Item's panel buttons", async () => {
    renderApp(tickets());
    await heading("Flaky build");
    expect(await within(focus()).findByRole("button", { name: "Send back" })).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Skip" })).toBeInTheDocument();
  });

  it("sends the item to the back of the queue and shows the next one, still counting it", async () => {
    const fake = tickets();
    renderApp(fake);
    await heading("Flaky build");
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    await skip();

    expect(await heading("Login page")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(actions(fake)).toEqual([]);
  });

  it("goes on to the Round's next open Question, and only the skipped one goes to the back of the whole queue", async () => {
    renderApp(world());
    await heading("Which colour?");
    await skip();
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(within(focus()).getByText("Question 2 of 2 · Round 1")).toBeInTheDocument();

    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    expect(await heading("Flaky build")).toBeInTheDocument();
  });

  it("keeps a skipped item behind one that arrives later", async () => {
    const fake = tickets();
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
    renderApp(tickets());
    await heading("Flaky build");
    await userEvent.keyboard("s");
    await userEvent.keyboard("k");
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });
});

describe("only skipped items left", () => {
  it("asks \"You skipped N, go through them again?\" instead of All clear, still counting them", async () => {
    renderApp(tickets());
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
    const fake = world();
    fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason === "needs_answer"));
    renderApp(fake);
    await heading("Which colour?");
    await skip();
    await heading("Which size?");
    await skip();

    expect(await within(focus()).findByText("You skipped 2, go through them again?")).toBeInTheDocument();
  });

  it("goes through them again from the front, in Needs Me order", async () => {
    renderApp(world());
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
    const fake = tickets();
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
    const fake = tickets();
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
    const fake = tickets();
    renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");

    expect(window.location.search).toBe("?view=focus");
    expect(localStorage.length).toBe(0);
    expect(actions(fake)).toEqual([]);
  });

  it("are forgotten on leaving Focus Mode", async () => {
    renderApp(tickets());
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    await userEvent.click(within(focus()).getByRole("button", { name: "Exit" }));
    await userEvent.click(within(await screen.findByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));

    expect(await heading("Flaky build")).toBeInTheDocument();
  });

  it("are forgotten on reload", async () => {
    const fake = tickets();
    const { unmount } = renderApp(fake);
    await heading("Flaky build");
    await skip();
    await heading("Login page");
    unmount();
    renderApp(fake);

    expect(await heading("Flaky build")).toBeInTheDocument();
  });
});
