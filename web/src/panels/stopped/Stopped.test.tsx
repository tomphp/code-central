// The Stopped panel, on the WASM Core through the real App: how long ago the
// Claim was stopped, then Resume, the Resume Command and Release, in that order.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../../api/core/core";
import { renderApp, scenario, settle } from "../../worldTesting";

const MINUTE = 60_000;

// stoppedWorld has a Factory Claim stopped 20 minutes ago.
async function stoppedWorld(over: { working_dir?: string } = {}) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Wire the API" });
  await s.agent(`${s.projectPath("acme")}/claim`, { ticket: id, session_id: "sess-42", factory: "code-central-factory", host: "laptop", ...over });
  await w.api.act("acme", id, "stop", {});
  w.advance(20 * MINUTE);
  return { w, id };
}

const panel = () => screen.getByRole("region", { name: "Stopped Claim" });

describe("the Stopped panel", () => {
  it("shows when the Claim was stopped, then Resume, the Resume Command and Release, in that order", async () => {
    const { w, id } = await stoppedWorld({ working_dir: "/work/acme-1" });
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const p = panel();
    expect(p).toHaveTextContent("Stopped 20m ago");

    const resume = within(p).getByRole("button", { name: "Resume" });
    const command = within(p).getByText("cd /work/acme-1 && claude --resume sess-42");
    const copy = within(p).getByRole("button", { name: "Copy resume command" });
    const release = within(p).getByRole("button", { name: "Release…" });
    const inOrder = (...nodes: HTMLElement[]) =>
      nodes.every((n, i) => i === 0 || nodes[i - 1].compareDocumentPosition(n) & Node.DOCUMENT_POSITION_FOLLOWING);
    expect(inOrder(resume, command, copy, release)).toBe(true);
  });

  it("copies the Resume Command", async () => {
    const user = userEvent.setup();
    const { w, id } = await stoppedWorld();
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    await user.click(within(panel()).getByRole("button", { name: "Copy resume command" }));
    expect(await navigator.clipboard.readText()).toBe("claude --resume sess-42");
    expect(within(panel()).getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("resumes the Claim, which leaves Needs Me", async () => {
    const user = userEvent.setup();
    const { w, id } = await stoppedWorld();
    await renderApp(w, `/?view=inbox&ticket=${id}`);

    await user.click(within(panel()).getByRole("button", { name: "Resume" }));
    await settle(w);

    expect(w.calls).toContain("resume");
    const t = await w.api.getTicket("acme", id);
    expect(t).toMatchObject({ state: "In Progress", stoppable: true });
    expect(t.needs_me_reason).toBeUndefined();
  });

  it("warns what releasing costs, then releases the Claim", async () => {
    const user = userEvent.setup();
    const { w, id } = await stoppedWorld();
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const p = panel();

    await user.click(within(p).getByRole("button", { name: "Release…" }));
    expect(within(p).getByRole("alert")).toHaveTextContent("a live session's next write will be rejected");
    await user.click(within(p).getByRole("button", { name: "Release Claim" }));
    await settle(w);

    expect(w.calls).toContain("release");
    expect((await w.api.getTicket("acme", id)).state).toBe("Ready");
  });

  it("lists the Ticket in Needs Me as Stopped", async () => {
    const { w, id } = await stoppedWorld();
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    const list = screen.getByRole("main", { name: "Tickets" });
    expect(within(list).getByRole("heading", { level: 3 })).toHaveTextContent("Stopped");
    expect(within(list).getByRole("link", { name: new RegExp(id) })).toHaveTextContent("Stopped 20m ago");
  });
});
