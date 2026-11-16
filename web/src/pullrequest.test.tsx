// The pull request link and state in the UI (code-central-388): on the
// Ticket page, the Spec's Ticket list and the Inbox rows, on the WASM Core.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import type { PullRequestState } from "./api/types";
import { allClear, renderApp, scenario } from "./worldTesting";

// world is an Accepted Ticket on Spec "Login" under Pull Request, pushed as
// its branch, with its pull request reported in state. Their IDs are the
// ones the writes returned.
async function world(state: PullRequestState = "open") {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  // A long-lived Spec's Tickets land one by one, each with a pull request.
  const spec = await s.spec("acme", { title: "Login", labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Login form", state: "Accepted", spec });
  await s.pullRequest("acme", ticket, state);
  return { w, spec, ticket };
}

describe("the pull request badge", () => {
  it("links to the pull request with its state on the Ticket page", async () => {
    const { w, ticket } = await world("conflicting");
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const pane = screen.getByRole("complementary", { name: "Ticket" });
    // In the Accepted panel and in the Spec's Ticket list beside it.
    const badges = within(pane).getAllByRole("link", { name: "Pull request 7, conflicting" });
    expect(badges).toHaveLength(2);
    for (const badge of badges) {
      expect(badge).toHaveAttribute("href", "https://github.com/acme/web/pull/7");
      expect(badge).toHaveTextContent("PR #7 conflicting");
    }
  });

  it("shows in the Spec's Ticket list", async () => {
    const { w, spec } = await world();
    await renderApp(w, `/?requirement=${spec}`);
    const row = screen.getAllByText("Login form").map((e) => e.closest("li")).find(Boolean)!;
    expect(within(row).getByRole("link", { name: "Pull request 7, open" })).toHaveAttribute("href", "https://github.com/acme/web/pull/7");
  });

  it("shows on an Inbox row without nesting a link in the row's link", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=inbox");
    const badge = screen.getByRole("link", { name: "Pull request 7, open" });
    expect(badge.tagName).toBe("SPAN");
    expect(badge.closest("a")).not.toBeNull();
    expect(badge).toHaveTextContent("PR #7 open");
  });

  it("is left out for a Ticket with no pull request", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const id = await s.ticket("acme", { title: "Plain", state: "Ready" });
    await renderApp(w, `/?view=inbox&ticket=${id}`);
    expect(screen.getByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
    expect(screen.queryByText(/^PR #/)).toBeNull();
  });
});
