// The pull request link and state in the UI (code-central-388): on the
// Ticket page, the Spec's Ticket list and the Inbox rows, against FakeApi.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { App } from "./App";
import { FakeApi } from "./api/fake";

function renderAt(fake: FakeApi, url: string) {
  window.history.replaceState(null, "", url);
  render(<App api={fake} now={() => fake.now} />);
}

// world is an Accepted Ticket acme-2 on Spec acme-1 under Pull Request,
// pushed as its branch, with its pull request reported in state.
async function world(state: "open" | "conflicting" | "closed" | "merged" = "open") {
  const fake = new FakeApi();
  fake.seedOptions({ focus_clear: "all_clear" });
  fake.addProject("acme", "Acme");
  fake.addSpec("acme", { title: "Login" });
  fake.addTicket("acme", { title: "Login form", state: "Accepted", spec: { id: "acme-1", title: "Login" } });
  await fake.reportPushRemote("acme", "git@github.com:acme/web.git");
  await fake.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
  await fake.reportPushedCommit("acme", "acme-2", "abc123", "acme-2");
  await fake.reportPullRequest("acme", {
    branch: "acme-2",
    number: 7,
    link: "https://github.com/acme/web/pull/7",
    state,
    ...(state === "merged" && { merge_commit: "def456" }),
  });
  return fake;
}

describe("the pull request badge", () => {
  it("links to the pull request with its state on the Ticket page", async () => {
    renderAt(await world("conflicting"), "/?view=inbox&ticket=acme-2");
    const pane = await screen.findByRole("complementary", { name: "Ticket" });
    const badge = await within(pane).findByRole("link", { name: "Pull request 7, conflicting" });
    expect(badge).toHaveAttribute("href", "https://github.com/acme/web/pull/7");
    expect(badge).toHaveTextContent("PR #7 conflicting");
  });

  it("shows in the Spec's Ticket list", async () => {
    renderAt(await world(), "/?requirement=acme-1");
    const row = (await screen.findByText("Login form")).closest("li")!;
    expect(await within(row).findByRole("link", { name: "Pull request 7, open" })).toHaveAttribute("href", "https://github.com/acme/web/pull/7");
  });

  it("shows on an Inbox row without nesting a link in the row's link", async () => {
    const fake = await world();
    fake.setNeedsMe([{ reason: "accepted_not_landed", tickets: [{ id: "acme-2" }] }]);
    renderAt(fake, "/?view=inbox");
    const badge = await screen.findByRole("link", { name: "Pull request 7, open" });
    expect(badge.tagName).toBe("SPAN");
    expect(badge.closest("a")).not.toBeNull();
    expect(badge).toHaveTextContent("PR #7 open");
  });

  it("is left out for a Ticket with no pull request", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Plain", state: "Ready" });
    renderAt(fake, "/?view=inbox&ticket=acme-1");
    await screen.findByRole("complementary", { name: "Ticket" });
    expect(screen.queryByText(/^PR #/)).toBeNull();
  });
});
