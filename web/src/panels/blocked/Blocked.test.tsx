// The Blocked panel against FakeApi: a Ticket stranded behind a Cancelled
// Blocker with no Replacement, and the three ways to unstick it.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import { panel } from "./panel";

function world(state: "Ready" | "Draft" = "Ready") {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Old plan", state: "Cancelled" }); // acme-1
  fake.addTicket("acme", { title: "Shipped", state: "Done" }); // acme-2
  fake.addTicket("acme", {
    title: "Build on it",
    state,
    blocked: true,
    blocked_by: ["acme-1", "acme-2"],
    blockers: [
      { id: "acme-1", title: "Old plan", state: "Cancelled", cleared: false },
      { id: "acme-2", title: "Shipped", state: "Done", cleared: false },
    ],
  }); // acme-3
  fake.setNeedsMe([
    { reason: "blocked_by_cancelled", tickets: [{ id: "acme-3", blocker: { id: "acme-1", title: "Old plan", state: "Cancelled", cleared: false } }] },
  ]);
  return fake;
}

async function renderPanel(fake: FakeApi, id = "acme-3") {
  const t = await fake.getTicket("acme", id);
  render(<panel.Component api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("region", { name: "Blocked" });
}

describe("the Blocked panel", () => {
  it("is the action panel for a Ticket blocked by a Cancelled Ticket", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-3");
    render(<App api={world()} now={() => new Date("2026-11-07T09:00:00Z")} />);
    const slot = await screen.findByRole("region", { name: "Action panel" });
    expect(await within(slot).findByRole("region", { name: "Blocked" })).toBeInTheDocument();
  });

  it("names the Cancelled Blocker and only that one", async () => {
    const p = await renderPanel(world());
    expect(p).toHaveTextContent("acme-1 Old plan");
    expect(p).toHaveTextContent(/Cancelled with no Replacement/);
    expect(p).not.toHaveTextContent("Shipped");
  });

  it("removes the Blocker", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Remove acme-1 as a Blocker" }));
    const t = await fake.getTicket("acme", "acme-3");
    expect(t.blocked_by).toEqual(["acme-2"]);
    expect(t.needs_me_reason).toBeUndefined();
  });

  it("removes the Cancelled Ticket a dead-end Replacement stands in for", async () => {
    const fake = world();
    fake.updateTicket("acme-3", {
      blocked_by: ["acme-2", "acme-1"],
      blockers: [{ id: "acme-2", title: "Shipped", state: "Done", cleared: false }, { id: "acme-4", title: "Also gone", state: "Cancelled", via: "acme-1", cleared: false }],
    });
    const p = await renderPanel(fake);
    expect(p).toHaveTextContent("acme-4 Also gone");
    expect(p).toHaveTextContent("replacing acme-1");
    await userEvent.click(within(p).getByRole("button", { name: "Remove acme-1 as a Blocker" }));
    expect((await fake.getTicket("acme", "acme-3")).blocked_by).toEqual(["acme-2"]);
  });

  it("moves the Ticket to Draft", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "To Draft" }));
    expect(fake.calls).toContain("move");
    expect((await fake.getTicket("acme", "acme-3")).state).toBe("Draft");
  });

  it("offers no To Draft for a Draft", async () => {
    const p = await renderPanel(world("Draft"));
    expect(within(p).queryByRole("button", { name: "To Draft" })).toBeNull();
    expect(within(p).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("cancels the Ticket with no Replacement", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Cancel" }));
    const t = await fake.getTicket("acme", "acme-3");
    expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
    expect(fake.calls).toContain("cancel");
  });

  it("shows a refused action's message", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    // Someone else removed the link first.
    fake.updateTicket("acme-3", { blocked_by: ["acme-2"] });
    await userEvent.click(within(p).getByRole("button", { name: "Remove acme-1 as a Blocker" }));
    expect(await within(p).findByRole("alert")).toHaveTextContent("acme-1 is not a Blocker of acme-3");
  });
});
