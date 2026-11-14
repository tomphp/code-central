// The CI failed panel (code-central-261), against FakeApi: the Ticket
// pane and the Focus Item both show the CI section with the failure
// options, and Land anyway records the human's choice; Send back to fix
// and New fix Ticket (code-central-264) carry a note prefilled with the
// failed run's link and failed jobs; and Re-run failed jobs
// (code-central-265) puts the Ticket back to waiting on CI.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import type { LandingMethod } from "../../api/types";
import { rowDetail } from "../../format";

// world has acme-1 Accepted under method with Push and CI Tracking on,
// its pushed commit abc1234567's run of ci.yml failed, in Needs Me as CI
// failed.
async function world(method: LandingMethod = "merge") {
  const fake = new FakeApi();
  fake.seedOptions({ focus_clear: "all_clear" });
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Login page", state: "Accepted", branch: "acme-1", commit: "abc" }); // acme-1
  if (method === "pull_request") await fake.reportPushRemote("acme", "git@github.com:acme/web.git");
  await fake.setLandingMethod("acme", { method, push: true, remote: "origin" });
  await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
  await fake.reportPushedCommit("acme", "acme-1", "abc1234567", "acme-1");
  await fake.reportCIRuns("acme", [
    {
      id: 1,
      attempt: 1,
      workflow: "ci.yml",
      name: "CI",
      branch: "main",
      commit: "abc1234567",
      status: "completed",
      conclusion: "failure",
      url: "https://github.com/acme/web/actions/runs/1",
      jobs: [
        { name: "test", conclusion: "failure", url: "https://github.com/acme/web/actions/runs/1/job/1" },
        { name: "lint", conclusion: "success", url: "https://github.com/acme/web/actions/runs/1/job/2" },
        { name: "build", conclusion: "timed_out", url: "https://github.com/acme/web/actions/runs/1/job/3" },
      ],
      ticket: "acme-1",
    },
  ]);
  fake.setNeedsMe([{ reason: "ci_failed", tickets: [{ id: "acme-1", ci_failure: { commit: "abc1234567", workflows: ["ci.yml"] } }] }]);
  return fake;
}

function renderAt(fake: FakeApi, url: string) {
  window.history.replaceState(null, "", url);
  render(<App api={fake} now={() => fake.now} />);
}

// pane is the Inbox's Ticket pane with acme-1 open.
async function pane(fake: FakeApi) {
  renderAt(fake, "/?view=inbox&ticket=acme-1");
  const p = screen.getByRole("complementary", { name: "Ticket" });
  await within(p).findByRole("region", { name: "CI failed" });
  return p;
}

describe("the CI failed panel", () => {
  it("is the Ticket pane's CI section, once, with each run's status, jobs and link", async () => {
    const p = await pane(await world());
    const panel = within(p).getByRole("region", { name: "CI failed" });
    const ci = within(panel).getByRole("region", { name: "CI" });
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI failed abc1234");
    expect(within(ci).getByRole("link", { name: "CI" })).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");
    expect(within(ci).getByRole("link", { name: "test" })).toBeInTheDocument();
    expect(within(p).getAllByRole("region", { name: "CI" })).toHaveLength(1);
  });

  it("lands anyway under Merge: the choice is recorded and the Ticket leaves Needs Me", async () => {
    const fake = await world();
    const p = await pane(fake);
    const options = within(p).getByRole("group", { name: "Failure options" });
    await userEvent.click(within(options).getByRole("button", { name: "Land anyway" }));

    expect(fake.actions()).toContain("land_anyway");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t).toMatchObject({ state: "Accepted", land_anyway: true });
    expect(t.needs_me_reason).toBeUndefined();
    expect((await fake.needsMe()).count).toBe(0);
    await waitFor(() => expect(within(p).getByRole("region", { name: "CI" })).toHaveTextContent("You accepted its failed CI result"));
    expect(within(p).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it("keeps the Accepted panel's Land and Cancel below it", async () => {
    const p = await pane(await world());
    const accepted = within(p).getByRole("region", { name: "Accepted" });
    expect(within(accepted).getByRole("button", { name: "Land" })).toBeInTheDocument();
    expect(within(accepted).getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("doesn't offer Land anyway under Branch, where the human's merge decides", async () => {
    const p = await pane(await world("branch"));
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(options).toHaveTextContent("your merge decides: merge acme-1 into the base");
    expect(within(p).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it("offers Re-run, Send back and New fix Ticket but never Land anyway under Pull Request", async () => {
    const fake = await world("pull_request");
    await fake.reportPullRequest("acme", { branch: "acme-1", number: 7, link: "https://github.com/acme/web/pull/7", state: "open" });
    const p = await pane(fake);
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(options).toHaveTextContent("your merge decides: merge pull request #7");
    expect(within(options).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "Send back to fix" })).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "New fix Ticket" })).toBeInTheDocument();
    expect(within(p).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it.each<LandingMethod>(["merge", "branch"])("re-runs the failed jobs under %s + Push: the Ticket waits on CI again", async (method) => {
    const fake = await world(method);
    const p = await pane(fake);
    const options = within(p).getByRole("group", { name: "Failure options" });
    await userEvent.click(within(options).getByRole("button", { name: "Re-run failed jobs" }));

    expect(fake.actions()).toContain("rerun_ci");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t).toMatchObject({ state: "Accepted", ci_result: "running" });
    expect(t).not.toHaveProperty("ci_failure_options");
    expect((await fake.needsMe()).count).toBe(0);
    await waitFor(() => expect(within(p).queryByRole("region", { name: "CI failed" })).toBeNull());
  });

  it("is the Focus Item's panel, and Re-run failed jobs works from there", async () => {
    const fake = await world();
    renderAt(fake, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    await userEvent.click(within(panel).getByRole("button", { name: "Re-run failed jobs" }));
    expect((await fake.getTicket("acme", "acme-1")).ci_result).toBe("running");
  });

  it("offers Re-run failed jobs on the Focus Item under Branch + Push", async () => {
    const fake = await world("branch");
    renderAt(fake, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it("is the Focus Item's panel, and Land anyway works from there", async () => {
    const fake = await world();
    renderAt(fake, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    expect(within(item).getByText("Focus: CI failed")).toBeInTheDocument();
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    await userEvent.click(within(panel).getByRole("button", { name: "Land anyway" }));
    expect((await fake.getTicket("acme", "acme-1")).land_anyway).toBe(true);
  });

  it("says in its Needs Me row what failed", async () => {
    const fake = await world();
    const row = (await fake.needsMe()).groups[0].tickets[0];
    expect(rowDetail(fake.now, row)).toBe("ci.yml failed");
  });

  it("prefills the note with the failed run's link and its failed jobs' names", async () => {
    const p = await pane(await world());
    const note = within(p).getByRole("textbox", { name: /Note for the fix/ });
    expect(note).toHaveValue(
      "CI failed at abc1234:\n\n- [CI](https://github.com/acme/web/actions/runs/1) (ci.yml): failed jobs build, test\n\nRead the failed jobs' logs with `gh run view 1 --log-failed`.",
    );
  });

  it("sends it back to fix with the note: it goes Ready and leaves Needs Me", async () => {
    const fake = await world();
    const p = await pane(fake);
    const note = within(p).getByRole("textbox", { name: /Note for the fix/ });
    await userEvent.type(note, " Also check the login test.");
    await userEvent.click(within(p).getByRole("button", { name: "Send back to fix" }));

    expect(fake.actions()).toContain("send_back");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
    expect(t.comments.at(-1)?.body).toMatch(/^CI failed at abc1234:[\s\S]*runs\/1\)[\s\S]*Also check the login test\.$/);
    expect((await fake.needsMe()).count).toBe(0);
  });

  it("opens a New fix Ticket on the CI Fixes Spec, and the failed Ticket keeps waiting, naming it", async () => {
    const fake = await world();
    const p = await pane(fake);
    await userEvent.click(within(p).getByRole("button", { name: "New fix Ticket" }));

    expect(fake.actions()).toContain("new_fix_ticket");
    const fix = await fake.getTicket("acme", "acme-3");
    expect(fix).toMatchObject({ state: "Ready", spec: { title: "CI Fixes" }, title: "Fix the CI failure of acme-1: Login page" });
    expect(fix.description).toContain("[CI](https://github.com/acme/web/actions/runs/1)");
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Accepted");
    expect(t.land_anyway).toBeFalsy();
    expect(t.comments.at(-1)?.body).toContain("acme-3");
    expect((await fake.needsMe()).count).toBe(0);
    await waitFor(() => expect(p).toHaveTextContent("acme-3 fixes this CI failure forward"));
  });

  it("needs a note for either fix", async () => {
    const fake = await world();
    const p = await pane(fake);
    await userEvent.clear(within(p).getByRole("textbox", { name: /Note for the fix/ }));
    expect(within(p).getByRole("button", { name: "Send back to fix" })).toBeDisabled();
    expect(within(p).getByRole("button", { name: "New fix Ticket" })).toBeDisabled();
  });

  it("offers both fixes under Branch too", async () => {
    const fake = await world("branch");
    const p = await pane(fake);
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(within(options).getByRole("button", { name: "Send back to fix" })).toBeInTheDocument();
    await userEvent.click(within(options).getByRole("button", { name: "New fix Ticket" }));
    expect((await fake.getTicket("acme", "acme-3")).state).toBe("Ready");
    expect((await fake.needsMe()).count).toBe(0);
  });

  it("offers both fixes on the Focus Item", async () => {
    const fake = await world();
    renderAt(fake, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByRole("button", { name: "New fix Ticket" })).toBeInTheDocument();
    await userEvent.click(within(panel).getByRole("button", { name: "Send back to fix" }));
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Ready");
  });
});

describe("the CI cancelled panel", () => {
  async function cancelledWorld() {
    const fake = await world();
    await fake.reportCIRuns("acme", [
      {
        id: 1,
        attempt: 1,
        workflow: "ci.yml",
        name: "CI",
        branch: "main",
        commit: "abc1234567",
        status: "completed",
        conclusion: "cancelled",
        url: "https://github.com/acme/web/actions/runs/1",
        jobs: [{ name: "test", conclusion: "cancelled", url: "https://github.com/acme/web/actions/runs/1/job/1" }],
        ticket: "acme-1",
      },
    ]);
    fake.setNeedsMe([{ reason: "ci_cancelled", tickets: [{ id: "acme-1" }] }]);
    return fake;
  }

  it("shows the cancelled result and run in grey, with Re-run, Land anyway and Send back but no New fix Ticket", async () => {
    renderAt(await cancelledWorld(), "/?view=inbox&ticket=acme-1");
    const p = screen.getByRole("complementary", { name: "Ticket" });
    const panel = await within(p).findByRole("region", { name: "CI cancelled" });
    const ci = within(panel).getByRole("region", { name: "CI" });
    expect(within(ci).getAllByText("cancelled").length).toBeGreaterThan(0);
    for (const el of within(ci).getAllByText("cancelled").filter((e) => e.className.includes("ci-"))) {
      expect(el).toHaveClass("ci-cancelled");
      expect(el).not.toHaveClass("ci-failed");
    }
    expect(within(ci).getByText("cancelled", { selector: ".ci-result" })).toHaveClass("ci-cancelled");
    expect(within(panel).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /Land anyway/ })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: /Send back/ })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: /New fix Ticket/ })).toBeNull();
  });
});
