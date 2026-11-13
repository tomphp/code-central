// The In Review panel, against FakeApi: what it shows and which action it
// calls with what payload, including Reject needing a comment.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import type { TicketView } from "../../api/types";
import { ReviewPanel } from "./panel";

const finished = "2026-11-07T08:00:00Z";

// world has acme-1 In Review, its finish summary body among other comments.
function world(body = "Added the login page\nBranch impl/7; check the redirect") {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Login page",
    state: "In Review",
    comments: [
      { number: 1, actor: "agent", time: "2026-11-07T07:00:00Z", claim: 1, body: "Starting on the form" },
      { number: 2, actor: "agent", time: finished, claim: 1, body },
      { number: 3, actor: "human", time: "2026-11-07T08:30:00Z", body: "Looking now" },
    ],
    moves: [
      { from: "Ready", to: "In Progress", actor: "agent", time: "2026-11-07T06:00:00Z" },
      { from: "In Progress", to: "In Review", actor: "agent", time: finished, claim: 1 },
    ],
  }); // acme-1
  fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-1", summary: "Added the login page" }] }]);
  return fake;
}

async function renderPanel(fake: FakeApi) {
  const ticket = await fake.getTicket("acme", "acme-1");
  render(<ReviewPanel api={fake} project="acme" ticket={ticket} now={fake.now} />);
  return screen.getByRole("region", { name: "In Review" });
}

// renderSummaryInApp opens acme-1 from world(body) in the inbox, through the
// whole app, and returns its Summary region.
async function renderSummaryInApp(body: string) {
  window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
  const fake = world(body);
  render(<App api={fake} now={() => fake.now} />);
  const pane = screen.getByRole("complementary", { name: "Ticket" });
  const panel = await within(pane).findByRole("region", { name: "In Review" });
  return within(panel).getByLabelText("Summary");
}

describe("the In Review panel", () => {
  it("shows the finish summary in full", async () => {
    const panel = await renderPanel(world());
    const summary = within(panel).getByLabelText("Summary");
    expect(summary).toHaveTextContent("Added the login page Branch impl/7; check the redirect");
    expect(panel).not.toHaveTextContent("Starting on the form");
    expect(panel).not.toHaveTextContent("Looking now");
  });

  it("accepts into Done", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));
    expect(fake.calls).toContain("accept");
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Done");
  });

  it("offers only Accept on a Ticket that isn't a spec Stage", async () => {
    const panel = await renderPanel(world());
    expect(within(panel).getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Split into Tickets" })).not.toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Implement" })).not.toBeInTheDocument();
  });

  describe("on a spec Stage", () => {
    async function specWorld(recommendation?: TicketView["recommendation"]) {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      const r = await fake.submitRequirement("acme", "Offline mode");
      const [grilling, spec] = r.stages.map((s) => s.id);
      fake.updateTicket(grilling, { state: "Done" });
      fake.updateTicket(spec, { state: "In Review", recommendation });
      return { fake, r, spec, tickets: r.stages[2].id };
    }
    async function renderSpec(fake: FakeApi, spec: string) {
      render(<ReviewPanel api={fake} project="acme" ticket={await fake.getTicket("acme", spec)} now={fake.now} />);
      return screen.getByRole("region", { name: "In Review" });
    }

    it("shows Split into Tickets and Implement in place of Accept", async () => {
      const { fake, spec } = await specWorld();
      const panel = await renderSpec(fake, spec);
      expect(within(panel).getByRole("button", { name: "Split into Tickets" })).toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Implement" })).toBeInTheDocument();
      expect(within(panel).queryByRole("button", { name: "Accept" })).not.toBeInTheDocument();
      expect(within(panel).getByRole("button", { name: "Reject" })).toBeInTheDocument();
    });

    it("with no recommendation, highlights neither choice", async () => {
      const { fake, spec } = await specWorld();
      const panel = await renderSpec(fake, spec);
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
      const { fake, spec } = await specWorld(recommendation);
      const panel = await renderSpec(fake, spec);
      const highlighted = within(panel).getByRole("button", { name: recommended });
      expect(highlighted).toHaveClass("primary");
      expect(highlighted).toHaveAccessibleDescription("Recommended");
      const plain = within(panel).getByRole("button", { name: other });
      expect(plain).not.toHaveClass("primary");
      expect(plain).not.toHaveAccessibleDescription();
    });

    it("Split into Tickets calls accept", async () => {
      const { fake, spec } = await specWorld();
      const panel = await renderSpec(fake, spec);
      await userEvent.click(within(panel).getByRole("button", { name: "Split into Tickets" }));
      expect(fake.calls).toContain("accept");
      expect(fake.calls).not.toContain("implement");
      expect((await fake.getTicket("acme", spec)).state).toBe("Done");
    });

    it("Implement calls implement: the tickets Stage is Cancelled with the new Ticket as Replacement", async () => {
      const { fake, spec, tickets } = await specWorld();
      const panel = await renderSpec(fake, spec);
      await userEvent.click(within(panel).getByRole("button", { name: "Implement" }));
      expect(fake.calls).toContain("implement");
      expect(fake.calls).not.toContain("accept");
      expect((await fake.getTicket("acme", spec)).state).toBe("Done");
      const cancelled = await fake.getTicket("acme", tickets);
      expect(cancelled.state).toBe("Cancelled");
      expect((await fake.getTicket("acme", cancelled.replacements[0])).title).toBe("Implement: Offline mode");
    });
  });

  it("won't send Reject without a comment", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    const reject = within(panel).getByRole("button", { name: "Reject" });
    expect(reject).toBeDisabled();
    await userEvent.type(within(panel).getByLabelText(/What to change/), "   ");
    expect(reject).toBeDisabled();
    await userEvent.click(reject);
    expect(fake.calls).not.toContain("reject");
  });

  it("rejects back to Ready with the comment", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.type(within(panel).getByLabelText(/What to change/), "The redirect loops");
    await userEvent.click(within(panel).getByRole("button", { name: "Reject" }));

    expect(fake.calls).toContain("reject");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "The redirect loops" });
  });

  it("says when no summary was recorded", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Moved by hand", state: "In Review" });
    const ticket = await fake.getTicket("acme", "acme-1");
    render(<ReviewPanel api={fake} project="acme" ticket={ticket} now={fake.now} />);
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
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    const fake = world();
    render(<App api={fake} now={() => fake.now} />);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByRole("region", { name: "In Review" })).toHaveTextContent("Added the login page");
  });
});
