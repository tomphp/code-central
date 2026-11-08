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
});
