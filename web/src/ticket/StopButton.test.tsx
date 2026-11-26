// The Stop button in the Ticket's header, in the Inbox's Ticket view and the
// Board's Ticket popup, on the WASM Core through the real App.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp, scenario, settle } from "../worldTesting";

async function runningWorld(claim: "factoryClaim" | "claim" = "factoryClaim") {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const id = await s.ticket("acme", { title: "Wire the API" });
  await s[claim]("acme", id, "sess-42");
  return { w, s, id };
}

const inbox = (w: World, id: string) => renderApp(w, `/?view=inbox&ticket=${id}`);
const header = () => document.querySelector<HTMLElement>(".ticket-head")!;

describe("the Stop button", () => {
  it("is offered while the Ticket is stoppable, in the Ticket view and in the Ticket popup", async () => {
    const { w, id } = await runningWorld();
    const view = await inbox(w, id);
    expect(within(header()).getByRole("button", { name: "Stop" })).toBeInTheDocument();
    view.unmount();

    await renderApp(w, `/?view=board&project=acme&popup=${id}`);
    const popup = screen.getByRole("dialog", { name: "Ticket popup" });
    expect(within(popup).getByRole("button", { name: "Stop" })).toBeInTheDocument();
  });

  it("is not offered for a Claim no Factory made", async () => {
    const { w, id } = await runningWorld("claim");
    await renderApp(w, `/?view=board&project=acme&popup=${id}`);
    const popup = screen.getByRole("dialog", { name: "Ticket popup" });
    expect(within(popup).getByRole("article", { name: "Ticket details" })).toBeInTheDocument();
    expect(within(popup).queryByRole("button", { name: "Stop" })).toBeNull();
  });

  it("is not offered once the Claim is Stopped", async () => {
    const { w, id } = await runningWorld();
    await w.api.act("acme", id, "stop", {});
    await inbox(w, id);
    expect(within(header()).queryByRole("button", { name: "Stop" })).toBeNull();
  });

  it("is offered on a Silent Claim as on any stoppable Ticket", async () => {
    const { w, id } = await runningWorld();
    w.advance(45 * 60_000);
    await inbox(w, id);
    expect(within(header()).getByRole("button", { name: "Stop" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Resume or release" })).toHaveTextContent("Release…");
  });

  it("confirms first, and stops nothing until it is confirmed", async () => {
    const user = userEvent.setup();
    const { w, id } = await runningWorld();
    await inbox(w, id);

    await user.click(within(header()).getByRole("button", { name: "Stop" }));
    expect(within(header()).getByRole("alert")).toHaveTextContent(`Stop ${id}'s session? The Claim is kept; resume it, take it over or release it later.`);
    expect(w.calls).not.toContain("stop");

    await user.click(within(header()).getByRole("button", { name: "Keep it running" }));
    expect(within(header()).queryByRole("alert")).toBeNull();
    expect(w.calls).not.toContain("stop");
  });

  it("calls stop once confirmed, and the Ticket reads Stopped", async () => {
    const user = userEvent.setup();
    const { w, id } = await runningWorld();
    await inbox(w, id);

    await user.click(within(header()).getByRole("button", { name: "Stop" }));
    await user.click(within(header()).getByRole("button", { name: "Stop session" }));
    await settle(w);

    expect(w.calls).toContain("stop");
    expect(await w.api.getTicket("acme", id)).toMatchObject({ state: "In Progress", needs_me_reason: "stopped" });
    expect(within(header()).queryByRole("button", { name: "Stop" })).toBeNull();
  });
});
