// CI unavailable in Needs Me (code-central-262): the real App against
// FakeApi. A Ticket waiting on CI with no word of it shows in its own
// group, its detail saying which: the CI Watcher not Watching, or no CI
// Runs reported for its pushed commit.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import { renderApp } from "../focus/focusTesting";

const minutesAgo = (fake: FakeApi, m: number) => new Date(fake.now.getTime() - m * 60_000).toISOString();

describe("CI unavailable in Needs Me", () => {
  it("shows its group and each row's cause", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Login page", state: "Accepted", ci_result: "running" }); // acme-1
    fake.addTicket("acme", { title: "Sign-up page", state: "Accepted", ci_result: "running" }); // acme-2
    fake.addTicket("acme", { title: "Profile page", state: "Accepted", ci_result: "running" }); // acme-3
    fake.setNeedsMe([
      {
        reason: "ci_unavailable",
        tickets: [
          { id: "acme-1", since: minutesAgo(fake, 12), ci_unavailable: { cause: "no_ci_runs", commit: "abc1234def", pushed: minutesAgo(fake, 12) } },
          {
            id: "acme-2",
            since: minutesAgo(fake, 15),
            ci_unavailable: { cause: "ci_watcher_not_watching", commit: "fed4321cba", pushed: minutesAgo(fake, 15), message: "gh is logged out" },
          },
          { id: "acme-3", since: minutesAgo(fake, 20), ci_unavailable: { cause: "ci_watcher_not_watching", commit: "0123456789", pushed: minutesAgo(fake, 20) } },
        ],
      },
    ]);
    renderApp(fake, "/?view=inbox");

    const list = screen.getByRole("main", { name: "Tickets" });
    const group = await within(list).findByRole("region", { name: "CI unavailable" });
    expect(within(group).getByRole("heading", { level: 3 })).toHaveTextContent("CI unavailable 3");
    expect(within(group).getByRole("link", { name: /acme-1/ })).toHaveTextContent("No CI Runs reported for abc1234 pushed 12m ago");
    expect(within(group).getByRole("link", { name: /acme-2/ })).toHaveTextContent(
      "CI Watcher not Watching: gh is logged out · fed4321 pushed 15m ago",
    );
    expect(within(group).getByRole("link", { name: /acme-3/ })).toHaveTextContent("CI Watcher not Watching · 0123456 pushed 20m ago");
  });
});
