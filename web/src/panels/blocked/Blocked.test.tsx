// The Blocked panel on the WASM Core: a Ticket stranded behind a Cancelled
// Blocker with no Replacement, and the three ways to unstick it.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../../api/core/core";
import { renderApp, renderLive, scenario, settle } from "../../worldTesting";
import { panel } from "./panel";

// world has a Cancelled and a Done Ticket and a Ticket blocked by both.
async function world(state: "Ready" | "Draft" = "Ready") {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const cancelled = await s.ticket("acme", { title: "Old plan", state: "Cancelled" });
  const done = await s.ticket("acme", { title: "Shipped", state: "Done" });
  const blocked = await s.ticket("acme", { title: "Build on it", state });
  await s.addBlocker("acme", blocked, cancelled);
  await s.addBlocker("acme", blocked, done);
  return { w, s, cancelled, done, blocked };
}

type Blocked = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, blocked }: Blocked) {
  const t = await w.api.getTicket("acme", blocked);
  await renderLive(w, <panel.Component project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("region", { name: "Blocked" });
}

describe("the Blocked panel", () => {
  it("is the action panel for a Ticket blocked by a Cancelled Ticket", async () => {
    const { w, blocked } = await world();
    await renderApp(w, `/?view=inbox&ticket=${blocked}`);
    const slot = screen.getByRole("region", { name: "Action panel" });
    expect(within(slot).getByRole("region", { name: "Blocked" })).toBeInTheDocument();
  });

  it("names the Cancelled Blocker and only that one", async () => {
    const x = await world();
    const p = await renderPanel(x);
    expect(p).toHaveTextContent(`${x.cancelled} Old plan`);
    expect(p).toHaveTextContent(/Cancelled with no Replacement/);
    expect(p).not.toHaveTextContent("Shipped");
  });

  it("removes the Blocker", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: `Remove ${x.cancelled} as a Blocker` }));
    const t = await x.w.api.getTicket("acme", x.blocked);
    expect(t.blocked_by).toEqual([x.done]);
    expect(t.needs_me_reason).toBeUndefined();
  });

  it("removes the Cancelled Ticket a dead-end Replacement stands in for", async () => {
    const x = await world();
    // The tickets Stage of a Requirement is Cancelled with its implementation
    // Ticket as Replacement; cancelling that leaves the dead end.
    const r = await x.w.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    const [grilling, spec, stage] = r.stages.map((s) => s.id);
    await x.w.api.act("acme", grilling, "move", { to_state: "Done" });
    await x.s.finish("acme", spec, "Wrote the Spec", { recommendation: "implement" });
    await x.w.api.act("acme", spec, "implement", {});
    const [gone] = (await x.w.api.getTicket("acme", stage)).replacements;
    await x.w.api.act("acme", gone, "cancel", {});
    const dependant = await x.s.ticket("acme", { title: "Depends on the stage", state: "Ready" });
    await x.s.addBlocker("acme", dependant, stage);

    const t = await x.w.api.getTicket("acme", dependant);
    await renderLive(x.w, <panel.Component project="acme" ticket={t} now={x.w.now()} />);
    const p = screen.getByRole("region", { name: "Blocked" });
    expect(p).toHaveTextContent(`${gone} Implement: Offline mode`);
    expect(p).toHaveTextContent(`replacing ${stage}`);
    await userEvent.click(within(p).getByRole("button", { name: `Remove ${stage} as a Blocker` }));
    expect((await x.w.api.getTicket("acme", dependant)).blocked_by).toEqual([]);
  });

  it("moves the Ticket to Draft", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "To Draft" }));
    expect(x.w.calls).toContain("move");
    expect((await x.w.api.getTicket("acme", x.blocked)).state).toBe("Draft");
  });

  it("offers no To Draft for a Draft", async () => {
    const p = await renderPanel(await world("Draft"));
    expect(within(p).queryByRole("button", { name: "To Draft" })).toBeNull();
    expect(within(p).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("cancels the Ticket with no Replacement", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Cancel" }));
    const t = await x.w.api.getTicket("acme", x.blocked);
    expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
    expect(x.w.calls).toContain("cancel");
  });

  it("shows a refused action's message", async () => {
    const x = await world();
    const p = await renderPanel(x);
    // Someone else removed the link first.
    await x.w.api.act("acme", x.blocked, "remove_blocker", { blocker: x.cancelled });
    await userEvent.click(within(p).getByRole("button", { name: `Remove ${x.cancelled} as a Blocker` }));
    await settle(x.w);
    expect(within(p).getByRole("alert")).toHaveTextContent(`${x.cancelled} is not a Blocker of ${x.blocked}`);
  });
});
