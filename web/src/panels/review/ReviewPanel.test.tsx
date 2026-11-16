// The In Review panel on the WASM Core: what it shows and which action it
// calls with what payload, including Reject needing a comment.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../../api/core/core";
import type { Scenario } from "../../api/scenario";
import type { TicketView } from "../../api/types";
import { renderApp, renderLive, scenario } from "../../worldTesting";
import { ReviewPanel } from "./panel";

// world has acme's Login page In Review, its finish summary body among
// other comments.
async function world(body = "Added the login page\nBranch impl/7; check the redirect") {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const id = await s.ticket("acme", { title: "Login page", state: "In Progress" });
  await s.agent(`${s.ticketPath("acme", id)}/comment`, { body: "Starting on the form" });
  w.advance(60 * 60_000);
  await s.inReview("acme", id, undefined, body);
  w.advance(30 * 60_000);
  await s.human(`${s.ticketPath("acme", id)}/comment`, { body: "Looking now" });
  return { w, s, id };
}

type Review = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, id }: Review) {
  const ticket = await w.api.getTicket("acme", id);
  await renderLive(w, <ReviewPanel project="acme" ticket={ticket} now={w.now()} />);
  return screen.getByRole("region", { name: "In Review" });
}

// renderSummaryInApp opens the Ticket from world(body) in the inbox, through the
// whole app, and returns its Summary region.
async function renderSummaryInApp(body: string) {
  const { w, id } = await world(body);
  await renderApp(w, `/?view=inbox&ticket=${id}`);
  const pane = screen.getByRole("complementary", { name: "Ticket" });
  const panel = within(pane).getByRole("region", { name: "In Review" });
  return within(panel).getByLabelText("Summary");
}

// inReview has the human move the Ticket to In Review by hand, which
// records no finish summary.
const inReview = (s: Scenario, id: string) => s.human(`${s.ticketPath("acme", id)}/move`, { to_state: "In Review" });

describe("the In Review panel", () => {
  // The kinds the Ticket can be: a Stage of a Requirement, the Spec merge
  // Ticket the Factory opens, or any other Ticket.
  it.each([
    ["grilling", "Grilling outcomes"],
    ["spec", "Spec recommendation"],
    ["tickets", "Tickets Stage"],
    ["implementation", "Implementation results"],
    ["merge", "Merge results"],
  ] as const)("names the %s Auto-accept kind beside its heading", async (kind, name) => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    let id: string;
    if (kind === "implementation") {
      id = await s.ticket("acme", { title: "Login page", state: "In Review" });
    } else if (kind === "merge") {
      const spec = await s.spec("acme");
      ({ ticket: id } = await s.agent(`${s.projectPath("acme")}/specs/${spec.slice(spec.lastIndexOf("-") + 1)}/open_spec_merge`, {
        conflicts: ["a.go"],
        base_tip: "b1",
        spec_branch_tip: "s1",
      }));
      await inReview(s, id);
    } else {
      const r = await w.api.submitRequirement("acme", "Offline mode");
      id = r.stages[["grilling", "spec", "tickets"].indexOf(kind)].id;
      await inReview(s, id);
    }
    const ticket = await w.api.getTicket("acme", id);
    expect(ticket.auto_accept_kind).toBe(kind);
    await renderLive(w, <ReviewPanel project="acme" ticket={ticket} now={w.now()} />);
    const panel = screen.getByRole("region", { name: "In Review" });
    expect(within(panel).getByText(`Auto-accept: ${name}`)).toBeInTheDocument();
    expect(within(panel).getByRole("heading", { name: "Review the finished work" })).toBeInTheDocument();
  });

  it("says never when the Ticket has no Auto-accept kind", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const id = await s.ticket("acme", { title: "Try a layout", labels: ["prototype"], state: "In Review" });
    const ticket = await w.api.getTicket("acme", id);
    expect(ticket.auto_accept_kind).toBeUndefined();
    await renderLive(w, <ReviewPanel project="acme" ticket={ticket} now={w.now()} />);
    expect(within(screen.getByRole("region", { name: "In Review" })).getByText("Auto-accept: never")).toBeInTheDocument();
  });

  it("shows the finish summary in full", async () => {
    const panel = await renderPanel(await world());
    const summary = within(panel).getByLabelText("Summary");
    expect(summary).toHaveTextContent("Added the login page Branch impl/7; check the redirect");
    expect(panel).not.toHaveTextContent("Starting on the form");
    expect(panel).not.toHaveTextContent("Looking now");
  });

  it("accepts into Accepted, to land", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));
    expect(x.w.calls).toContain("accept");
    expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Accepted");
  });

  it("offers only Accept on a Ticket that isn't a spec Stage", async () => {
    const panel = await renderPanel(await world());
    expect(within(panel).getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Split into Tickets" })).not.toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Implement" })).not.toBeInTheDocument();
  });

  describe("on a spec Stage", () => {
    // specWorld has Offline mode's spec Stage In Review: finished with the
    // recommendation if there is one, else moved there by hand.
    async function specWorld(recommendation?: TicketView["recommendation"]) {
      const w = await newWorld();
      const s = scenario(w);
      await s.project("acme", "Acme");
      const r = await w.api.submitRequirement("acme", "Offline mode");
      const [grilling, spec] = r.stages.map((s) => s.id);
      await w.api.act("acme", grilling, "move", { to_state: "Done" });
      if (recommendation) await s.finish("acme", spec, "Wrote the Spec", { recommendation });
      else await inReview(s, spec);
      return { w, r, spec, tickets: r.stages[2].id };
    }
    async function renderSpec({ w }: Awaited<ReturnType<typeof specWorld>>, spec: string) {
      await renderLive(w, <ReviewPanel project="acme" ticket={await w.api.getTicket("acme", spec)} now={w.now()} />);
      return screen.getByRole("region", { name: "In Review" });
    }

    it("shows Split into Tickets and Implement in place of Accept", async () => {
      const x = await specWorld();
      const panel = await renderSpec(x, x.spec);
      expect(within(panel).getByRole("button", { name: "Split into Tickets" })).toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Implement" })).toBeInTheDocument();
      expect(within(panel).queryByRole("button", { name: "Accept" })).not.toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Reject" })).toBeInTheDocument();
    });

    it("with no recommendation, highlights neither choice", async () => {
      const x = await specWorld();
      const panel = await renderSpec(x, x.spec);
      for (const name of ["Split into Tickets", "Implement"]) {
        const button = within(panel).getByRole("button", { name });
        expect(button).toHaveClass("primary");
        expect(button).not.toHaveAccessibleDescription();
      }
      expect(panel).not.toHaveTextContent("Recommended");
    });

    it.each([
      ["implement", "Implement", "Split into Tickets"],
      ["split", "Split into Tickets", "Implement"],
    ] as const)("highlights the recommended choice: %s", async (recommendation, recommended, other) => {
      const x = await specWorld(recommendation);
      const panel = await renderSpec(x, x.spec);
      const highlighted = within(panel).getByRole("button", { name: recommended });
      expect(highlighted).toHaveClass("primary");
      expect(highlighted).toHaveAccessibleDescription("Recommended");
      expect(highlighted.nextElementSibling).toHaveTextContent("Recommended");
      const plain = within(panel).getByRole("button", { name: other });
      expect(plain).not.toHaveClass("primary");
      expect(plain).not.toHaveAccessibleDescription();
    });

    it("Split into Tickets calls accept", async () => {
      const x = await specWorld();
      const panel = await renderSpec(x, x.spec);
      await userEvent.click(within(panel).getByRole("button", { name: "Split into Tickets" }));
      expect(x.w.calls).toContain("accept");
      expect(x.w.calls).not.toContain("implement");
      expect((await x.w.api.getTicket("acme", x.spec)).state).toBe("Done");
    });

    it("Implement calls implement: the tickets Stage is Cancelled with the new Ticket as Replacement", async () => {
      const x = await specWorld();
      const panel = await renderSpec(x, x.spec);
      await userEvent.click(within(panel).getByRole("button", { name: "Implement" }));
      expect(x.w.calls).toContain("implement");
      expect(x.w.calls).not.toContain("accept");
      expect((await x.w.api.getTicket("acme", x.spec)).state).toBe("Done");
      const cancelled = await x.w.api.getTicket("acme", x.tickets);
      expect(cancelled.state).toBe("Cancelled");
      expect((await x.w.api.getTicket("acme", cancelled.replacements[0])).title).toBe("Implement: Offline mode");
    });
  });

  it("won't send Reject without a comment", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    const reject = within(panel).getByRole("button", { name: "Reject" });
    expect(reject).toBeDisabled();
    await userEvent.type(within(panel).getByLabelText(/What to change/), "   ");
    expect(reject).toBeDisabled();
    await userEvent.click(reject);
    expect(x.w.calls).not.toContain("reject");
  });

  it("rejects back to Ready with the comment", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.type(within(panel).getByLabelText(/What to change/), "The redirect loops");
    await userEvent.click(within(panel).getByRole("button", { name: "Reject" }));

    expect(x.w.calls).toContain("reject");
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Ready");
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "The redirect loops" });
  });

  it("says when no summary was recorded", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const id = await s.ticket("acme", { title: "Moved by hand" });
    await inReview(s, id);
    const ticket = await w.api.getTicket("acme", id);
    await renderLive(w, <ReviewPanel project="acme" ticket={ticket} now={w.now()} />);
    expect(screen.getByRole("region", { name: "In Review" })).toHaveTextContent("No finish summary");
  });

  it("renders the finish summary as Markdown", async () => {
    const summary = await renderSummaryInApp("Added **the login page**\n\n- form\n- redirect\n\nRun `make test`.");
    expect(within(summary).getByRole("strong")).toHaveTextContent("the login page");
    expect(within(summary).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["form", "redirect"]);
    expect(within(summary).getByRole("code")).toHaveTextContent("make test");
  });

  it("shows raw HTML in the finish summary as text", async () => {
    const summary = await renderSummaryInApp('Done <img src=x onerror="alert(1)"> here');
    expect(within(summary).queryByRole("img")).toBeNull();
    expect(summary).toHaveTextContent('Done <img src=x onerror="alert(1)"> here');
  });

  it("is the action panel for an In Review Ticket in the inbox", async () => {
    const { w, id } = await world();
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(pane).getByRole("region", { name: "In Review" })).toHaveTextContent("Added the login page");
  });
});
