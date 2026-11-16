// CI unavailable in Needs Me (code-central-262): the real App on a WASM
// Core world. A pushed commit the CI gate holds with no word of its CI
// shows in its own group, its detail saying which: the CI Watcher not
// Watching, or no CI Runs reported for the commit.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { renderApp, scenario } from "../worldTesting";

// pushed has a Ticket accepted and its commit pushed, returning its ID.
async function pushed(s: Scenario, key: string, title: string, commit: string) {
  const id = await s.ticket(key, { title });
  await s.accepted(key, id, { branch: "impl", commit });
  await s.pushed(key, id, commit);
  return id;
}

describe("CI unavailable in Needs Me", () => {
  it("shows its group and each row's cause", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    await s.project("beta", "Beta");
    await s.project("gamma", "Gamma");
    // acme's CI Watcher is Watching, reporting runs for another commit.
    await pushed(s, "acme", "Sign-up page", "1111111111");
    await s.reportCIWatcherStatus("acme", { state: "watching" });
    await s.ci("acme", "acme-2", "passed", "1111111111");
    await pushed(s, "acme", "Login page", "abc1234def");
    // beta's is in error.
    await pushed(s, "beta", "Profile page", "fed4321cba");
    await s.reportCIWatcherStatus("beta", { state: "error", problem: "logged_out", message: "gh is logged out", fix: "Run gh auth login." });
    // gamma's has not reported.
    await pushed(s, "gamma", "Settings page", "0123456789");
    w.advance(20 * 60_000);
    await renderApp(w, "/?view=inbox");

    const list = screen.getByRole("main", { name: "Tickets" });
    const group = within(list).getByRole("region", { name: "CI unavailable" });
    expect(within(group).getByRole("heading", { level: 3 })).toHaveTextContent("CI unavailable 3");
    expect(within(group).getByRole("link", { name: /abc1234/ })).toHaveTextContent("No CI Runs reported for abc1234 pushed 20m ago");
    expect(within(group).getByRole("link", { name: /fed4321/ })).toHaveTextContent("CI Watcher not Watching: gh is logged out · fed4321 pushed 20m ago");
    expect(within(group).getByRole("link", { name: /0123456/ })).toHaveTextContent("CI Watcher not Watching · 0123456 pushed 20m ago");
  });
});
