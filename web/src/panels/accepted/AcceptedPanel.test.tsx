// The Accepted panel (ticket 71, ADR 0006) on the WASM Core: accepted
// work waiting to land, with Land (asking for the commit) and Cancel.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../../api/core/core";
import { renderApp, renderLive, scenario, settle } from "../../worldTesting";
import { AcceptedPanel } from "./panel";

// world has acme's Login page Accepted, under Merge unless method says,
// landing on its own rather than with its Spec.
async function world(method: "merge" | "branch" = "merge") {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  await s.human(`${s.projectPath("acme")}/set_landing_method`, { method, push: false, remote: "origin", land_per_spec: false });
  const id = await s.ticket("acme", { title: "Login page", state: "Accepted" });
  return { w, s, id };
}

type Accepted = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, id }: Accepted) {
  const ticket = await w.api.getTicket("acme", id);
  await renderLive(w, <AcceptedPanel project="acme" ticket={ticket} now={w.now()} />);
  return screen.getByRole("region", { name: "Accepted" });
}

describe("the Accepted panel", () => {
  it("shows the branch and head commit waiting to land", async () => {
    const panel = await renderPanel(await world());
    expect(panel).toHaveTextContent("impl");
    expect(panel).toHaveTextContent("abc123");
  });

  it("won't Land without the commit that put the work into the base", async () => {
    const panel = await renderPanel(await world());
    const land = within(panel).getByRole("button", { name: "Land" });
    expect(land).toBeDisabled();
    await userEvent.type(within(panel).getByLabelText(/Commit in the base/), "   ");
    expect(land).toBeDisabled();
  });

  it("lands into Done with the commit", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.type(within(panel).getByLabelText(/Commit in the base/), " def456 ");
    await userEvent.click(within(panel).getByRole("button", { name: "Land" }));

    expect(x.w.calls).toContain("land");
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Done");
    expect(t.moves!.at(-1)).toMatchObject({ from: "Accepted", to: "Done", actor: "human", via: "merge", commit: "def456" });
  });

  it("cancels", async () => {
    const x = await world();
    const panel = await renderPanel(x);
    await userEvent.click(within(panel).getByRole("button", { name: "Cancel" }));
    expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Cancelled");
  });

  it("is the action panel for an Accepted Ticket in the inbox", async () => {
    const { w, id } = await world();
    await renderApp(w, `/?ticket=${id}&state=Accepted`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(pane).getByRole("region", { name: "Accepted" })).toBeInTheDocument();
    const nav = screen.getByRole("navigation", { name: "Filters" });
    expect(within(nav).getByRole("button", { name: "Accepted 1" })).toBeInTheDocument();
  });

  it("shows Accepted but not landed in Needs Me, with how long since the accept, and this panel", async () => {
    const { w, id } = await world();
    w.advance(25 * 60_000);
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(within(list).getByRole("link", { name: new RegExp(id) })).toHaveTextContent("Accepted 25m ago, not landed");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("Accepted but not landed");
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    expect(within(pane).getByRole("region", { name: "Accepted" })).toBeInTheDocument();
  });

  it("under Branch, tells the human to merge the branch, keeping Land and Cancel", async () => {
    const x = await world("branch");
    const panel = await renderPanel(x);
    expect(panel).toHaveTextContent(`merge impl into the base and the Factory lands it, or Land it yourself with the commit`);
    expect(within(panel).getByRole("button", { name: "Land" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("under Merge, says nothing about merging it yourself", async () => {
    const panel = await renderPanel(await world());
    expect(panel).not.toHaveTextContent("into the base and the Factory");
    expect(panel).toHaveTextContent("It is Done once it is merged into the base and landed.");
  });

  describe("To merge options", () => {
    it("under Branch, offers only Send back to fix, which needs the note, and sends it back", async () => {
      const x = await world("branch");
      const panel = await renderPanel(x);
      expect(within(panel).queryByRole("button", { name: "New fix Ticket" })).toBeNull();
      const send = within(panel).getByRole("button", { name: "Send back to fix" });
      expect(send).toBeDisabled();
      await userEvent.type(within(panel).getByLabelText(/Note for the fix/), "Rename it");
      await userEvent.click(send);
      await settle(x.w);
      expect(x.w.actions()).toContain("send_back");
      expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Ready");
    });

    it("offers nothing under Merge, where the Ticket isn't To merge", async () => {
      const panel = await renderPanel(await world());
      expect(within(panel).queryByRole("button", { name: "Send back to fix" })).toBeNull();
      expect(within(panel).queryByRole("button", { name: "New fix Ticket" })).toBeNull();
      expect(within(panel).queryByLabelText(/Note for the fix/)).toBeNull();
    });

    it("on a Spec's pull-request lead offers New fix Ticket, not Send back to fix, and says where the new Ticket is", async () => {
      const x = await world();
      await x.s.pullRequest("acme", x.id);
      await x.s.human(`${x.s.projectPath("acme")}/set_landing_method`, { method: "pull_request", push: false, remote: "origin", land_per_spec: true });
      const panel = await renderPanel(x);
      expect(within(panel).queryByRole("button", { name: "Send back to fix" })).toBeNull();
      const fix = within(panel).getByRole("button", { name: "New fix Ticket" });
      expect(fix).toBeDisabled();
      await userEvent.type(within(panel).getByLabelText(/Note for the fix/), "Reviewers want a rename");
      await userEvent.click(fix);
      await settle(x.w);
      expect(x.w.actions()).toContain("new_fix_ticket");
      const link = within(panel).getByRole("link", { name: /^\S+-\d+$/ });
      const opened = await x.w.api.getTicket("acme", link.textContent!);
      expect(opened).toMatchObject({ state: "Ready", title: expect.any(String) });
    });

    it("shows the core's refusal", async () => {
      const x = await world("branch");
      const ticket = { ...(await x.w.api.getTicket("acme", x.id)), to_merge_options: ["new_fix_ticket" as const] };
      await renderLive(x.w, <AcceptedPanel project="acme" ticket={ticket} now={x.w.now()} />);
      const panel = screen.getByRole("region", { name: "Accepted" });
      await userEvent.type(within(panel).getByLabelText(/Note for the fix/), "x");
      await userEvent.click(within(panel).getByRole("button", { name: "New fix Ticket" }));
      await settle(x.w);
      expect(within(panel).getByRole("alert")).toBeInTheDocument();
    });
  });

  it("shows To merge in Needs Me, naming the branch, with this panel", async () => {
    const { w, id } = await world("branch");
    w.advance(5 * 60_000);
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(within(list).getByRole("link", { name: new RegExp(id) })).toHaveTextContent("Accepted 5m ago: merge impl into the base");
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("To merge");
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    const panel = within(pane).getByRole("region", { name: "Accepted" });
    expect(panel).toHaveTextContent("and the Factory lands it");
  });
});
