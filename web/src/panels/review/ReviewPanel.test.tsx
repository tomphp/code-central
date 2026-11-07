// The In Review panel, against FakeApi: what it shows and which action it
// calls with what payload, including Reject needing a comment.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import { ReviewPanel } from "./panel";

const finished = "2026-11-07T08:00:00Z";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Login page",
    state: "In Review",
    comments: [
      { number: 1, actor: "agent", time: "2026-11-07T07:00:00Z", claim: 1, body: "Starting on the form" },
      { number: 2, actor: "agent", time: finished, claim: 1, body: "Added the login page\nBranch impl/7; check the redirect" },
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

  it("is the action panel for an In Review Ticket in the inbox", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    const fake = world();
    render(<App api={fake} now={() => fake.now} />);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByRole("region", { name: "In Review" })).toHaveTextContent("Added the login page");
  });
});
