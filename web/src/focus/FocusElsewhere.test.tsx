// Dealt with elsewhere, in Focus Mode and the Focus popup: the real App on
// a WASM Core world, at ?view=focus, or with the popup over another view.
// An item on screen that goes away without the human acting on it here
// moves on with a brief note; one the human acted on moves on silently; a
// refused action keeps the item on screen with its alert. A read Round
// with no open Question left waits, in both, for Needs Me to catch up.
import { render, screen, within, act, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import type { World } from "../api/core/core";
import type { Api } from "../api/api";
import type { Scenario } from "../api/scenario";
import { acme, allClearOn, focus, renderApp } from "./worldTesting";

const heading = (name: string) => within(focus()).findByRole("heading", { name });
const note = () => within(focus()).queryByText("Dealt with elsewhere");

// cancel has the human cancel the Ticket elsewhere.
const cancel = (s: Scenario, id: string) => act(() => s.human(`${s.ticketPath("acme", id)}/cancel`, {}));

// touch writes to the Ticket, so Core sends its change notice.
const touch = (s: Scenario, id: string) => act(() => s.human(`${s.projectPath("acme")}/add_labels`, { target: id, labels: ["touched"] }));

// tickets is acme with All clear on, then Flaky build Failed and Login
// page In Review, in that Needs Me order.
async function tickets() {
  const { w, s } = await acme();
  await allClearOn(w);
  const failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
  const review = await s.ticket("acme", { title: "Login page", state: "In Review" });
  return { w, s, failed, review };
}

// world is tickets with Pick a colour waiting on Round 1 (Questions 1 and
// 2 open) ahead of them.
async function world() {
  const t = await tickets();
  const colour = await t.s.ticket("acme", { title: "Pick a colour" });
  await t.s.askRound("acme", colour, [
    { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
    { title: "Which size?", body: "S or M" },
  ]);
  return { ...t, colour };
}

// answer has the human answer a Question elsewhere.
const answer = (s: Scenario, id: string, question: number, text: string) => act(() => s.human(`${s.ticketPath("acme", id)}/answer`, { question, text }));

// held is w's Api with Needs Me held at what it said when hold was called,
// until release: Needs Me not yet caught up with the Tickets. getTicket
// is a spy.
function held(w: World) {
  let hold: Awaited<ReturnType<Api["needsMe"]>> | undefined;
  const getTicket = vi.fn((...a: Parameters<Api["getTicket"]>) => w.api.getTicket(...a));
  const api = new Proxy(w.api, {
    get: (t, k: string) => (k === "needsMe" ? () => (hold ? Promise.resolve(hold) : t.needsMe()) : k === "getTicket" ? getTicket : t[k as keyof Api]),
  });
  return {
    api,
    getTicket,
    hold: async () => {
      hold = await w.api.needsMe();
    },
    release: () => {
      hold = undefined;
    },
  };
}

function renderHeld(w: World, api: Api, url: string) {
  window.history.replaceState(null, "", url);
  return render(<App api={api} now={w.now} />);
}

describe("dealt with elsewhere", () => {
  it("moves on with a brief note when the Ticket on screen leaves its reason in Needs Me", async () => {
    const { w, s, failed } = await tickets();
    renderApp(w);
    await within(focus()).findByRole("button", { name: "Send back" });
    await cancel(s, failed);

    expect(await heading("Login page")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("moves on with the note when the Question on screen is answered elsewhere, losing a half-typed answer", async () => {
    const { w, s, colour } = await world();
    renderApp(w);
    await heading("Which colour?");
    const box = within(focus()).getByRole("textbox");
    await userEvent.clear(box);
    await userEvent.type(box, "Gre");
    await answer(s, colour, 1, "Green");

    expect(await heading("Which size?")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
    expect(within(focus()).getByRole("textbox")).toHaveValue("");
  });

  it("keeps the note while the next item, a Round, is read into its first Question", async () => {
    // Core lists Needs Answer first, longest-waiting first: the next
    // Round is another Ticket's.
    const { w, s } = await acme();
    await allClearOn(w);
    const colour = await s.ticket("acme", { title: "Pick a colour" });
    await s.askRound("acme", colour, [{ title: "Which colour?", body: "Red or blue" }]);
    w.advance(60_000);
    const shape = await s.ticket("acme", { title: "Pick a shape" });
    await s.askRound("acme", shape, [{ title: "Which shape?", body: "Round or square" }]);
    renderApp(w);
    await heading("Which colour?");
    await cancel(s, colour);

    expect(await heading("Which shape?")).toBeInTheDocument();
    expect(within(focus()).getByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("isn't on All clear, which stays calm, when the last item goes elsewhere", async () => {
    const { w, s } = await acme();
    await allClearOn(w);
    const failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    renderApp(w);
    await within(focus()).findByRole("button", { name: "Send back" });
    await cancel(s, failed);

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("goes once the human moves on from the next item", async () => {
    const { w, s, failed } = await tickets();
    renderApp(w);
    await within(focus()).findByRole("button", { name: "Send back" });
    await cancel(s, failed);
    await within(focus()).findByText("Dealt with elsewhere");
    await userEvent.click(await within(focus()).findByRole("button", { name: "Accept" }));

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    // Core's change notice and the Accept's reply race, so the note may be
    // a moment going.
    await waitFor(() => expect(note()).toBeNull());
  });

  it("isn't shown when the human skips the item", async () => {
    const { w } = await tickets();
    renderApp(w);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

    expect(await heading("Login page")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("isn't shown when a new item arrives ahead of the one on screen", async () => {
    const { w, s, review } = await tickets();
    await s.ticket("acme", { title: "Quiet session", state: "In Progress" });
    renderApp(w);
    await within(focus()).findByRole("button", { name: "Send back" });
    // Silent after 30 minutes, which Core lists ahead of Failed; a change
    // to another Ticket makes the App look again.
    w.advance(31 * 60_000);
    await touch(s, review);

    await within(focus()).findByText("3 left");
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(note()).toBeNull();
  });
});

describe("the human's own action", () => {
  it("moves a Ticket item on with no note", async () => {
    const { w } = await tickets();
    renderApp(w);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Send back" }));

    expect(await heading("Login page")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("moves a Ticket item on with no note after Unlink, a write that isn't a Ticket action", async () => {
    // closed_externally comes last in Needs Me, so it is the only item.
    const { w, s } = await acme();
    await allClearOn(w);
    const link = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };
    const closed = await s.incomingIssue("acme", await s.spec("acme"), "Closed upstream", link);
    await s.integration(`${s.projectPath("acme")}/integration/set_external_state`, { target: closed, external_state: "closed" });
    renderApp(w);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Unlink" }));

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("moves a Question on with no note after Answer, or Dismiss", async () => {
    const { w } = await world();
    renderApp(w);
    await heading("Which colour?");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(note()).toBeNull();

    // The Round is complete, so the Ticket comes back Answered.
    await userEvent.click(within(focus()).getByRole("button", { name: "Dismiss" }));
    expect(await within(focus()).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
    expect(note()).toBeNull();
  });
});

describe("a refused action", () => {
  it("keeps a Question on screen with the form's alert, and the human can try again", async () => {
    const { w } = await world();
    renderApp(w);
    await heading("Which colour?");
    const box = within(focus()).getByRole("textbox");
    await userEvent.clear(box);
    await userEvent.type(box, "   ");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));

    expect(await within(focus()).findByRole("alert")).toHaveTextContent("an answer needs text");
    expect(within(focus()).getByRole("heading", { name: "Which colour?" })).toBeInTheDocument();
    expect(note()).toBeNull();

    await userEvent.clear(box);
    await userEvent.type(box, "Green");
    await userEvent.click(within(focus()).getByRole("button", { name: "Answer" }));
    expect(await heading("Which size?")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("keeps a Ticket item on screen with its panel's alert", async () => {
    const { w, s, failed } = await tickets();
    // The Ticket is cancelled before Needs Me says so, so the service refuses Cancel.
    const h = held(w);
    await h.hold();
    await s.human(`${s.ticketPath("acme", failed)}/cancel`, {});
    renderHeld(w, h.api, "/?view=focus");
    await userEvent.click(await within(focus()).findByRole("button", { name: "Cancel" }));

    expect(await within(focus()).findByRole("alert")).toHaveTextContent(`${failed} is Cancelled, so cancel is not allowed`);
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });

  it("doesn't count as the human's action when the item then goes elsewhere", async () => {
    const { w, s, failed, review } = await tickets();
    const h = held(w);
    await h.hold();
    await s.human(`${s.ticketPath("acme", failed)}/cancel`, {});
    renderHeld(w, h.api, "/?view=focus");
    await userEvent.click(await within(focus()).findByRole("button", { name: "Cancel" }));
    await within(focus()).findByRole("alert");
    h.release();
    await touch(s, review);

    expect(await heading("Login page")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });
});

describe("in the Focus popup", () => {
  // popupOn turns Focus Pop-up on in w.
  const popupOn = (s: Scenario) => s.human("/api/set_options", { focus_popup: "on" });
  const popup = () => screen.getByRole("dialog", { name: "Focus popup" });
  const inPopup = (name: string) => waitFor(() => within(popup()).getByRole("heading", { name }));
  const popupNote = () => within(popup()).queryByText("Dealt with elsewhere");

  it("moves on with a brief note when the Ticket on screen leaves its reason in Needs Me", async () => {
    const { w, s, failed } = await tickets();
    await popupOn(s);
    renderApp(w, "/?view=inbox");
    await within(await screen.findByRole("dialog", { name: "Focus popup" })).findByRole("button", { name: "Send back" });
    await cancel(s, failed);

    expect(await inPopup("Login page")).toBeInTheDocument();
    expect(await within(popup()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("moves a card's Question on with the note when it's answered elsewhere", async () => {
    const { w, s, colour } = await world();
    renderApp(w, `/?view=board&project=acme&popup=${colour}`);
    await inPopup("Which colour?");
    await answer(s, colour, 1, "Green");

    expect(await inPopup("Which size?")).toBeInTheDocument();
    expect(await within(popup()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("moves on with no note after the human's own action", async () => {
    const { w, s } = await tickets();
    await popupOn(s);
    renderApp(w, "/?view=inbox");
    await inPopup("Flaky build");
    await userEvent.click(await within(popup()).findByRole("button", { name: "Send back" }));

    expect(await inPopup("Login page")).toBeInTheDocument();
    expect(popupNote()).toBeNull();
  });

  it("moves a Question on with no note after Answer", async () => {
    const { w, s } = await world();
    await popupOn(s);
    renderApp(w, "/?view=inbox");
    await inPopup("Which colour?");
    await userEvent.click(within(popup()).getByRole("button", { name: "Answer" }));

    expect(await inPopup("Which size?")).toBeInTheDocument();
    expect(popupNote()).toBeNull();
  });

  it("doesn't count a refused action as the human's when the item then goes elsewhere", async () => {
    const { w, s, failed, review } = await tickets();
    await popupOn(s);
    const h = held(w);
    await h.hold();
    await s.human(`${s.ticketPath("acme", failed)}/cancel`, {});
    renderHeld(w, h.api, "/?view=inbox");
    await inPopup("Flaky build");
    await userEvent.click(await within(popup()).findByRole("button", { name: "Cancel" }));
    await within(popup()).findByRole("alert");
    h.release();
    await touch(s, review);

    expect(await inPopup("Login page")).toBeInTheDocument();
    expect(await within(popup()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });
});

describe("a read Round with no open Question left, before Needs Me catches up", () => {
  // answeredRound is a world with only its Round, every Question of which
  // is answered while Needs Me, held, still lists the Round: Needs Me
  // hasn't caught up yet.
  async function answeredRound() {
    const { w, s } = await acme();
    await allClearOn(w);
    const colour = await s.ticket("acme", { title: "Pick a colour" });
    await s.askRound("acme", colour, [
      { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
      { title: "Which size?", body: "S or M" },
    ]);
    const h = held(w);
    await h.hold();
    await s.answerRound("acme", colour, ["Yes", "Yes"]);
    return { w, s, colour, h };
  }

  // waitsThenCatchesUp checks that region shows Loading… while it waits,
  // without reading the Round again and again, then shows the Ticket as
  // Answered once Needs Me catches up.
  async function waitsThenCatchesUp({ s, colour, h }: Awaited<ReturnType<typeof answeredRound>>, region: () => HTMLElement) {
    await waitFor(() => expect(h.getTicket).toHaveBeenCalled());
    await waitFor(() => expect(region()).toHaveTextContent("Loading…"));
    await act(() => new Promise((r) => setTimeout(r, 50)));
    expect(h.getTicket.mock.calls.length).toBeLessThan(5);
    expect(within(region()).queryByRole("article", { name: "Focus Item" })).toBeNull();

    h.release();
    await touch(s, colour);
    expect(await within(region()).findByRole("region", { name: "Resume or release" })).toBeInTheDocument();
  }

  it("waits in Focus Mode", async () => {
    const a = await answeredRound();
    renderHeld(a.w, a.h.api, "/?view=focus");
    await waitsThenCatchesUp(a, focus);
  });

  it("waits in the Focus popup, which stays open", async () => {
    const a = await answeredRound();
    await a.s.human("/api/set_options", { focus_popup: "on" });
    renderHeld(a.w, a.h.api, "/?view=inbox");
    await waitsThenCatchesUp(a, () => screen.getByRole("dialog", { name: "Focus popup" }));
  });
});
