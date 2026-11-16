// Needs Me and Focus Mode under Pull Request (code-central-391): To merge
// with the pull request's link and "conflicting", Pull request closed with
// Send back and Cancel, and no Land anyway, on the WASM Core.
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import type { PullRequestState } from "./api/types";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario, settle } from "./worldTesting";

const PR_LINK = "https://github.com/acme/web/pull/7";

// world is an Accepted Ticket under Pull Request with its pull request
// reported in state, which Needs Me carries as To merge or Pull request closed.
async function world(state: PullRequestState) {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  // A long-lived Spec's Tickets land one by one, each with a pull request.
  const spec = await s.spec("acme", { labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Login form", state: "Accepted", spec });
  await s.pullRequest("acme", ticket, state);
  return { w, ticket };
}

const item = () => within(focus()).getByRole("article", { name: "Focus Item" });

describe("To merge under Pull Request", () => {
  it("says conflicting and links the pull request in the Needs Me row", async () => {
    const { w } = await world("conflicting");
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(w.now(), row)).toMatch(/merge pull request #7, conflicting$/);
    expect(row.pull_request).toMatchObject({ link: PR_LINK, state: "conflicting" });
  });

  it("is a Focus Item linking the pull request, offering Send back to fix with a note", async () => {
    const { w, ticket } = await world("conflicting");
    await renderApp(w);
    const it = item();
    expect(within(it).getByText("Focus: To merge")).toBeInTheDocument();
    const link = within(it).getByRole("link", { name: "Pull request 7, conflicting" });
    expect(link).toHaveAttribute("href", PR_LINK);

    const send = within(it).getByRole("button", { name: "Send back to fix" });
    expect(send).toBeDisabled();
    await userEvent.type(within(it).getByLabelText(/Note for the fix/), "Merge main in and resolve src/a.ts");
    await userEvent.click(send);
    await settle(w);
    expect((await w.api.getTicket("acme", ticket)).state).toBe("Ready");
    expect(w.actions()).toContain("send_back");
  });
});

describe("Pull request closed", () => {
  it("is a Focus Item linking the pull request with Send back and Cancel", async () => {
    const { w, ticket } = await world("closed");
    await renderApp(w);
    const it = item();
    expect(within(it).getByText("Focus: Pull request closed")).toBeInTheDocument();
    expect(within(it).getByRole("heading", { name: "Pull request closed" })).toBeInTheDocument();
    expect(within(it).getByRole("link", { name: "Pull request 7, closed" })).toHaveAttribute("href", PR_LINK);
    expect(within(it).queryByRole("button", { name: /Land anyway/ })).toBeNull();

    const send = within(it).getByRole("button", { name: "Send back" });
    expect(send).toBeDisabled();
    await userEvent.type(within(it).getByLabelText(/Note for the next agent/), "Redo the form");
    await userEvent.click(send);
    await settle(w);
    expect((await w.api.getTicket("acme", ticket)).state).toBe("Ready");
  });

  it("cancels", async () => {
    const { w, ticket } = await world("closed");
    await renderApp(w);
    const it = item();
    await userEvent.click(within(it).getByRole("button", { name: "Cancel" }));
    await settle(w);
    expect((await w.api.getTicket("acme", ticket)).state).toBe("Cancelled");
  });

  it("describes the row with its pull request", async () => {
    const { w } = await world("closed");
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(w.now(), row)).toBe("Pull request #7 was closed without merging: send the work back or cancel it");
  });
});
