// The Accepted panel (ticket 71, ADR 0006), against FakeApi: accepted
// work waiting to land, with Land (asking for the commit) and Cancel.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import { AcceptedPanel } from "./panel";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", {
    title: "Login page",
    state: "Accepted",
    branch: "acme-1",
    commit: "abc123",
    moves: [
      { from: "In Progress", to: "In Review", actor: "agent", time: "2026-11-07T08:00:00Z", claim: 1 },
      { from: "In Review", to: "Accepted", actor: "human", time: "2026-11-07T09:00:00Z" },
    ],
  }); // acme-1
  return fake;
}

async function renderPanel(fake: FakeApi) {
  const ticket = await fake.getTicket("acme", "acme-1");
  render(<AcceptedPanel api={fake} project="acme" ticket={ticket} now={fake.now} />);
  return screen.getByRole("region", { name: "Accepted" });
}

describe("the Accepted panel", () => {
  it("shows the branch and head commit waiting to land", async () => {
    const panel = await renderPanel(world());
    expect(panel).toHaveTextContent("acme-1");
    expect(panel).toHaveTextContent("abc123");
  });

  it("won't Land without the commit that put the work into the base", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    const land = within(panel).getByRole("button", { name: "Land" });
    expect(land).toBeDisabled();
    await userEvent.type(within(panel).getByLabelText(/Commit in the base/), "   ");
    expect(land).toBeDisabled();
  });

  it("lands into Done with the commit", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.type(within(panel).getByLabelText(/Commit in the base/), " def456 ");
    await userEvent.click(within(panel).getByRole("button", { name: "Land" }));

    expect(fake.calls).toContain("land");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Done");
    expect(t.moves.at(-1)).toMatchObject({ from: "Accepted", to: "Done", actor: "human", via: "merge", commit: "def456" });
  });

  it("cancels", async () => {
    const fake = world();
    const panel = await renderPanel(fake);
    await userEvent.click(within(panel).getByRole("button", { name: "Cancel" }));
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Cancelled");
  });

  it("is the action panel for an Accepted Ticket in the inbox", async () => {
    window.history.replaceState(null, "", "/?ticket=acme-1&state=Accepted");
    const fake = world();
    render(<App api={fake} now={() => fake.now} />);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByRole("region", { name: "Accepted" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Filters" });
    expect(await within(nav).findByRole("button", { name: "Accepted 1" })).toBeInTheDocument();
  });

  it("shows Accepted but not landed in Needs Me, with how long since the accept, and this panel", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    const fake = world();
    fake.setNeedsMe([
      { reason: "accepted_not_landed", tickets: [{ id: "acme-1", since: new Date(fake.now.getTime() - 25 * 60_000).toISOString() }] },
    ]);
    render(<App api={fake} now={() => fake.now} />);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(await within(list).findByRole("link", { name: /acme-1/ })).toHaveTextContent("Accepted 25m ago, not landed");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("Accepted but not landed");
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(await within(pane).findByRole("region", { name: "Accepted" })).toBeInTheDocument();
  });

  it("under Branch, tells the human to merge the branch, keeping Land and Cancel", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "to_merge", tickets: [{ id: "acme-1", branch: "acme-1" }] }]);
    const panel = await renderPanel(fake);
    expect(panel).toHaveTextContent("merge acme-1 into the base and the Factory lands it, or Land it yourself with the commit");
    expect(within(panel).getByRole("button", { name: "Land" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("under Merge, says nothing about merging it yourself", async () => {
    const panel = await renderPanel(world());
    expect(panel).not.toHaveTextContent("merge acme-1 into the base");
    expect(panel).toHaveTextContent("It is Done once it is merged into the base and landed.");
  });

  it("shows To merge in Needs Me, naming the branch, with this panel", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    const fake = world();
    fake.setNeedsMe([
      { reason: "to_merge", tickets: [{ id: "acme-1", branch: "acme-1", since: new Date(fake.now.getTime() - 5 * 60_000).toISOString() }] },
    ]);
    render(<App api={fake} now={() => fake.now} />);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(await within(list).findByRole("link", { name: /acme-1/ })).toHaveTextContent("Accepted 5m ago: merge acme-1 into the base");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("To merge");
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    const panel = await within(pane).findByRole("region", { name: "Accepted" });
    expect(panel).toHaveTextContent("and the Factory lands it");
  });
});
