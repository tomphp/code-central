// Pull requests per Repository in the UI (Spec code-central-848): Pull
// request closed names each closed pull request's Repository and offers
// no Cancel once a sibling has merged, and the Pull Request Watcher's
// status names each Repository it can't watch, on the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import type { PullRequestState } from "./api/types";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario, settle } from "./worldTesting";

// world is acme with the Repository api added after the Home Repository,
// under Pull Request, with an Accepted Ticket that changed both and whose
// pull requests are in home and api states.
async function world(home: PullRequestState, api: PullRequestState) {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.repository("acme", "api");
  const spec = await s.spec("acme", { title: "Endpoints", labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Add an endpoint", state: "Ready", spec });
  await s.accepted("acme", ticket, { branch: ticket, commit: "9a8b7c6d5e4f", repositories: [{ name: "api", branch: ticket, commit: "1234567890ab" }] });
  await s.pullRequest("acme", ticket, home);
  await s.pullRequestIn("acme", ticket, "api", api, 4);
  return { w, s, ticket };
}

const item = () => within(focus()).getByRole("article", { name: "Focus Item" });

describe("Pull request closed across Repositories", () => {
  it("names each closed pull request's Repository and still offers Cancel while none has merged", async () => {
    const { w, ticket } = await world("open", "closed");
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(row.reason).toBe("pull_request_closed");
    expect(rowDetail(w.now(), row)).toBe("Pull request #4 in api was closed without merging: send the work back or reopen it");

    await renderApp(w);
    const closed = within(item()).getByRole("list", { name: "Closed pull requests" });
    expect(within(closed).getAllByRole("listitem").map((li) => li.getAttribute("aria-label"))).toEqual(["api"]);
    expect(within(closed).getByRole("link", { name: "Pull request 4, closed" })).toHaveAttribute("href", "https://github.com/acme/api/pull/4");
    await userEvent.click(within(item()).getByRole("button", { name: "Cancel" }));
    await settle(w);
    expect((await w.api.getTicket("acme", ticket)).state).toBe("Cancelled");
  });

  it("offers no Cancel once a sibling has merged, only Send back", async () => {
    const { w } = await world("closed", "merged");
    await renderApp(w);
    expect(within(item()).getByText("Focus: Pull request closed")).toBeInTheDocument();
    expect(within(item()).queryByRole("button", { name: "Cancel" })).toBeNull();
    expect(within(item()).getByRole("button", { name: "Send back" })).toBeInTheDocument();
    expect(item()).toHaveTextContent("Already merged in api, so it can't be cancelled");
  });
});

describe("the Pull Request Watcher across Repositories", () => {
  const landing = () => within(screen.getByRole("main", { name: "Project Options" })).getByRole("group", { name: "Default Landing" });

  it("watches every Repository and names each one it can't watch with its fix", async () => {
    const { w, s } = await world("open", "open");
    await renderApp(w, "/?view=project-options&project=acme");
    const watcher = () => within(landing()).getByRole("region", { name: "Pull Request Watcher" });

    await s.reportPullRequestWatcher("acme", { state: "watching" });
    expect(watcher()).toHaveTextContent("Pull Request Watcher: Watching its 2 Repositories.");

    await s.reportPullRequestWatcher("acme", {
      state: "error",
      problem: "pat_missing",
      message: "api: ADO_PAT isn't set",
      fix: "api: set ADO_PAT",
      repositories: [{ repository: "api", problem: "pat_missing", message: "ADO_PAT isn't set", fix: "Set ADO_PAT for the service." }],
    });
    expect(watcher()).toHaveTextContent("Error in 1 Repository. It carries on watching the others");
    const failing = within(watcher()).getByRole("list", { name: "Failing Repositories" });
    const api = within(failing).getByRole("listitem", { name: "api" });
    expect(api).toHaveTextContent("api: ADO_PAT isn't set");
    expect(api).toHaveTextContent("Set ADO_PAT for the service.");
  });
});
