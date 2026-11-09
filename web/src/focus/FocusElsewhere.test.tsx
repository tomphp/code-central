// Dealt with elsewhere, in Focus Mode: the real App against FakeApi, at
// ?view=focus. An item on screen that goes away without the human acting
// on it here moves on with a brief note; one the human acted on moves on
// silently; a refused action keeps the item on screen with its alert.
import { screen, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { allClearOn, change, focus, renderApp } from "./focusTesting";
import type { RoundView } from "../api/types";

// world has acme-1 waiting on Round 1 (Questions 1 and 2 open), then
// acme-2 Failed and acme-3 In Review, in that Needs Me order.
function world() {
  const fake = new FakeApi();
  allClearOn(fake);
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

const heading = (name: string) => within(focus()).findByRole("heading", { name });
const note = () => within(focus()).queryByText("Dealt with elsewhere");
const without = (fake: FakeApi, id: string) => fake.needsMeGroups.map((g) => ({ ...g, tickets: g.tickets.filter((r) => r.id !== id) })).filter((g) => g.tickets.length);

describe("dealt with elsewhere", () => {
  it("moves on with a brief note when the Ticket on screen leaves its reason in Needs Me", async () => {
    const fake = tickets();
    renderApp(fake);
    await within(focus()).findByRole("button", { name: "Send back" });
    change(fake, without(fake, "acme-2"), "acme-2");

    expect(await heading("Login page")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("moves on with the note when the Question on screen is answered elsewhere, losing a half-typed answer", async () => {
    const fake = world();
    renderApp(fake);
    await heading("Which colour?");
    const box = within(focus()).getByRole("textbox");
    await userEvent.clear(box);
    await userEvent.type(box, "Gre");
    await fake.act("acme", "acme-1", "answer", { question: 1, text: "Green" });

    expect(await heading("Which size?")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
    expect(within(focus()).getByRole("textbox")).toHaveValue("");
  });

  it("keeps the note while the next item, a Round, is read into its first Question", async () => {
    const fake = world();
    fake.setNeedsMe([fake.needsMeGroups[1], fake.needsMeGroups[0], fake.needsMeGroups[2]]);
    renderApp(fake);
    await within(focus()).findByRole("button", { name: "Send back" });
    change(fake, without(fake, "acme-2"), "acme-2");

    expect(await heading("Which colour?")).toBeInTheDocument();
    expect(within(focus()).getByText("Dealt with elsewhere")).toBeInTheDocument();
  });

  it("isn't on All clear, which stays calm, when the last item goes elsewhere", async () => {
    const fake = tickets();
    fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason === "failed"));
    renderApp(fake);
    await within(focus()).findByRole("button", { name: "Send back" });
    change(fake, [], "acme-2");

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("goes once the human moves on from the next item", async () => {
    const fake = tickets();
    renderApp(fake);
    await within(focus()).findByRole("button", { name: "Send back" });
    change(fake, without(fake, "acme-2"), "acme-2");
    await within(focus()).findByText("Dealt with elsewhere");
    await userEvent.click(await within(focus()).findByRole("button", { name: "Accept" }));

    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("isn't shown when the human skips the item", async () => {
    renderApp(tickets());
    await userEvent.click(await within(focus()).findByRole("button", { name: "Skip" }));

    expect(await heading("Login page")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("isn't shown when a new item arrives ahead of the one on screen", async () => {
    const fake = tickets();
    renderApp(fake);
    await within(focus()).findByRole("button", { name: "Send back" });
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-4
    change(fake, [{ reason: "silent_claim", tickets: [{ id: t.id }] }, ...fake.needsMeGroups], t.id);

    await within(focus()).findByText("3 left");
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(note()).toBeNull();
  });
});

describe("the human's own action", () => {
  it("moves a Ticket item on with no note", async () => {
    const fake = tickets();
    fake.setNeedsMe([...fake.needsMeGroups].reverse());
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Accept" }));

    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("moves a Ticket item on with no note after Unlink, a write that isn't a Ticket action", async () => {
    const fake = tickets();
    const link = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7", external_state: "closed", detached: false } as const;
    const t = fake.addTicket("acme", { title: "Closed upstream", state: "Ready", link }); // acme-4
    fake.setNeedsMe([{ reason: "closed_externally", tickets: [{ id: t.id, link: t.link }] }, ...fake.needsMeGroups]);
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Unlink" }));
    // The service takes it out of Needs Me and sends its notice.
    change(fake, without(fake, t.id), t.id);

    expect(await heading("Flaky build")).toBeInTheDocument();
    expect(note()).toBeNull();
  });

  it("moves a Question on with no note after Answer, or Dismiss", async () => {
    renderApp(world());
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
    const fake = world();
    renderApp(fake);
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
    const fake = tickets();
    // The Ticket is finished, so the service refuses Cancel.
    fake.updateTicket("acme-2", { state: "Done" });
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Cancel" }));

    expect(await within(focus()).findByRole("alert")).toHaveTextContent("cancel needs an unfinished Ticket");
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });

  it("doesn't count as the human's action when the item then goes elsewhere", async () => {
    const fake = tickets();
    fake.updateTicket("acme-2", { state: "Done" });
    renderApp(fake);
    await userEvent.click(await within(focus()).findByRole("button", { name: "Cancel" }));
    await within(focus()).findByRole("alert");
    change(fake, without(fake, "acme-2"), "acme-2");

    expect(await heading("Login page")).toBeInTheDocument();
    expect(await within(focus()).findByText("Dealt with elsewhere")).toBeInTheDocument();
  });
});
