// The CI failed panel (code-central-261), on the WASM Core: the Ticket
// pane and the Focus Item both show the CI section with the failure
// options, and Land anyway records the human's choice; Send back to fix
// and New fix Ticket (code-central-264) carry a note prefilled with the
// failed run's link and failed jobs; and Re-run failed jobs
// (code-central-265) puts the Ticket back to waiting on CI.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { newWorld, type World } from "../../api/core/core";
import { scenario } from "../../api/scenario";
import type { CIRunView, LandingMethod } from "../../api/types";
import { rowDetail } from "../../format";

const COMMIT = "abc1234567";
const RUN_URL = "https://github.com/acme/web/actions/runs/1";

// world has Login page Accepted under method with Push and CI Tracking
// on, landing on its own, its pushed commit abc1234567's run of ci.yml
// concluded as conclusion says, in Needs Me as CI failed (or cancelled).
async function world(method: LandingMethod = "merge", conclusion: "failure" | "cancelled" = "failure") {
  const w = await newWorld();
  const s = scenario(w);
  await w.api.setOptions({ focus_clear: "all_clear" });
  await s.project("acme", "Acme");
  if (method === "pull_request") await s.reportPushRemote("acme", "git@github.com:acme/web.git");
  await s.human(`${s.projectPath("acme")}/set_landing_method`, { method, push: true, remote: "origin", land_per_spec: false });
  await s.human(`${s.projectPath("acme")}/set_ci_tracking`, { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
  const id = await s.ticket("acme", { title: "Login page" });
  await s.accepted("acme", id, { branch: id, commit: "abc" });
  await s.reportPushedCommit("acme", id, COMMIT, id);
  const jobs: CIRunView["jobs"] =
    conclusion === "cancelled"
      ? [{ name: "test", conclusion: "cancelled", url: `${RUN_URL}/job/1` }]
      : [
          { name: "test", conclusion: "failure", url: `${RUN_URL}/job/1` },
          { name: "lint", conclusion: "success", url: `${RUN_URL}/job/2` },
          { name: "build", conclusion: "timed_out", url: `${RUN_URL}/job/3` },
        ];
  await s.reportCIRuns("acme", [
    { id: 1, attempt: 1, workflow: "ci.yml", name: "CI", branch: "main", commit: COMMIT, status: "completed", conclusion, url: RUN_URL, jobs, ticket: id },
  ]);
  return { w, s, id };
}

type Failed = Awaited<ReturnType<typeof world>>;

function renderAt(w: World, url: string) {
  window.history.replaceState(null, "", url);
  render(<App api={w.api} now={w.now} />);
}

// pane is the Inbox's Ticket pane with the Ticket open.
async function pane({ w, id }: Failed) {
  renderAt(w, `/?view=inbox&ticket=${id}`);
  const p = screen.getByRole("complementary", { name: "Ticket" });
  await within(p).findByRole("region", { name: "CI failed" });
  return p;
}

// fixOf is the Ticket New fix Ticket opened.
async function fixOf({ w }: Failed) {
  const [fix] = (await w.api.findTickets("acme")).filter((t) => t.title.startsWith("Fix the CI failure"));
  return w.api.getTicket("acme", fix.id);
}

describe("the CI failed panel", () => {
  it("is the Ticket pane's CI section, once, with each run's status, jobs and link", async () => {
    const p = await pane(await world());
    const panel = within(p).getByRole("region", { name: "CI failed" });
    const ci = within(panel).getByRole("region", { name: "CI" });
    expect(within(ci).getByRole("heading")).toHaveTextContent("CI failed abc1234");
    expect(within(ci).getByRole("link", { name: "CI" })).toHaveAttribute("href", RUN_URL);
    expect(within(ci).getByRole("link", { name: "test" })).toBeInTheDocument();
    expect(within(p).getAllByRole("region", { name: "CI" })).toHaveLength(1);
  });

  it("lands anyway under Merge: the choice is recorded and the Ticket leaves Needs Me", async () => {
    const x = await world();
    const p = await pane(x);
    const options = within(p).getByRole("group", { name: "Failure options" });
    await userEvent.click(within(options).getByRole("button", { name: "Land anyway" }));

    await waitFor(() => expect(x.w.actions()).toContain("land_anyway"));
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t).toMatchObject({ state: "Accepted", land_anyway: true });
    expect(t.needs_me_reason).toBeUndefined();
    expect((await x.w.api.needsMe()).count).toBe(0);
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
    const x = await world("branch");
    const p = await pane(x);
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(options).toHaveTextContent(`your merge decides: merge ${x.id} into the base`);
    expect(within(p).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it("offers Re-run, Send back and New fix Ticket but never Land anyway under Pull Request", async () => {
    const x = await world("pull_request");
    await x.s.reportPullRequest("acme", { branch: x.id, number: 7, link: "https://github.com/acme/web/pull/7", state: "open" });
    const p = await pane(x);
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(options).toHaveTextContent("your merge decides: merge pull request #7");
    expect(within(options).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "Send back to fix" })).toBeInTheDocument();
    expect(within(options).getByRole("button", { name: "New fix Ticket" })).toBeInTheDocument();
    expect(within(p).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it.each<LandingMethod>(["merge", "branch"])("re-runs the failed jobs under %s + Push: the Ticket waits on CI again", async (method) => {
    const x = await world(method);
    const p = await pane(x);
    const options = within(p).getByRole("group", { name: "Failure options" });
    await userEvent.click(within(options).getByRole("button", { name: "Re-run failed jobs" }));

    await waitFor(() => expect(x.w.actions()).toContain("rerun_ci"));
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t).toMatchObject({ state: "Accepted", ci_result: "running" });
    expect(t).not.toHaveProperty("ci_failure_options");
    // Under Branch the Ticket is still to merge, by the human.
    expect((await x.w.api.needsMe()).groups.map((g) => g.reason)).toEqual(method === "branch" ? ["to_merge"] : []);
    await waitFor(() => expect(within(p).queryByRole("region", { name: "CI failed" })).toBeNull());
  });

  it("is the Focus Item's panel, and Re-run failed jobs works from there", async () => {
    const x = await world();
    renderAt(x.w, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    await userEvent.click(within(panel).getByRole("button", { name: "Re-run failed jobs" }));
    await waitFor(async () => expect((await x.w.api.getTicket("acme", x.id)).ci_result).toBe("running"));
  });

  it("offers Re-run failed jobs on the Focus Item under Branch + Push", async () => {
    const x = await world("branch");
    renderAt(x.w, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByRole("button", { name: "Re-run failed jobs" })).toBeInTheDocument();
    expect(within(panel).queryByRole("button", { name: "Land anyway" })).toBeNull();
  });

  it("is the Focus Item's panel, and Land anyway works from there", async () => {
    const x = await world();
    renderAt(x.w, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    expect(within(item).getByText("Focus: CI failed")).toBeInTheDocument();
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    await userEvent.click(within(panel).getByRole("button", { name: "Land anyway" }));
    await waitFor(async () => expect((await x.w.api.getTicket("acme", x.id)).land_anyway).toBe(true));
  });

  it("says in its Needs Me row what failed", async () => {
    const { w, id } = await world();
    const row = (await w.api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(w.now(), row)).toBe(`${id} · ci.yml failed`);
  });

  it("prefills the note with the failed run's link and its failed jobs' names", async () => {
    const p = await pane(await world());
    const note = within(p).getByRole("textbox", { name: /Note for the fix/ });
    expect(note).toHaveValue(
      `CI failed at abc1234:\n\n- [CI](${RUN_URL}) (ci.yml): failed jobs build, test\n\nRead the failed jobs' logs with \`gh run view 1 --log-failed\`.`,
    );
  });

  it("sends it back to fix with the note: it goes Ready and leaves Needs Me", async () => {
    const x = await world();
    const p = await pane(x);
    const note = within(p).getByRole("textbox", { name: /Note for the fix/ });
    await userEvent.type(note, " Also check the login test.");
    await userEvent.click(within(p).getByRole("button", { name: "Send back to fix" }));

    await waitFor(() => expect(x.w.actions()).toContain("send_back"));
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Ready");
    expect(t.comments!.at(-1)?.body).toMatch(/^CI failed at abc1234:[\s\S]*runs\/1\)[\s\S]*Also check the login test\.$/);
    expect((await x.w.api.needsMe()).count).toBe(0);
  });

  it("opens a New fix Ticket on the CI Fixes Spec, and the failed Ticket keeps waiting, naming it", async () => {
    const x = await world();
    const p = await pane(x);
    await userEvent.click(within(p).getByRole("button", { name: "New fix Ticket" }));

    await waitFor(() => expect(x.w.actions()).toContain("new_fix_ticket"));
    const fix = await fixOf(x);
    expect(fix).toMatchObject({ state: "Ready", spec: { title: "CI Fixes" }, title: `Fix the CI failure of ${x.id}: Login page` });
    expect(fix.description).toContain(`[CI](${RUN_URL})`);
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Accepted");
    expect(t.land_anyway).toBeFalsy();
    expect(t.comments!.at(-1)?.body).toContain(fix.id);
    expect((await x.w.api.needsMe()).count).toBe(0);
    await waitFor(() => expect(p).toHaveTextContent(`${fix.id} fixes this CI failure forward`));
  });

  it("needs a note for either fix", async () => {
    const p = await pane(await world());
    await userEvent.clear(within(p).getByRole("textbox", { name: /Note for the fix/ }));
    expect(within(p).getByRole("button", { name: "Send back to fix" })).toBeDisabled();
    expect(within(p).getByRole("button", { name: "New fix Ticket" })).toBeDisabled();
  });

  it("offers both fixes under Branch too", async () => {
    const x = await world("branch");
    const p = await pane(x);
    const options = within(p).getByRole("group", { name: "Failure options" });
    expect(within(options).getByRole("button", { name: "Send back to fix" })).toBeInTheDocument();
    await userEvent.click(within(options).getByRole("button", { name: "New fix Ticket" }));
    await waitFor(async () => expect((await fixOf(x)).state).toBe("Ready"));
    // The Ticket is still to merge, by the human.
    expect((await x.w.api.needsMe()).groups.map((g) => g.reason)).toEqual(["to_merge"]);
  });

  it("offers both fixes on the Focus Item", async () => {
    const x = await world();
    renderAt(x.w, "/?view=focus");
    const item = await screen.findByRole("article", { name: "Focus Item" });
    const panel = await within(item).findByRole("region", { name: "CI failed" });
    expect(within(panel).getByRole("button", { name: "New fix Ticket" })).toBeInTheDocument();
    await userEvent.click(within(panel).getByRole("button", { name: "Send back to fix" }));
    await waitFor(async () => expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Ready"));
  });
});

describe("the CI cancelled panel", () => {
  it("shows the cancelled result and run in grey, with Re-run, Land anyway and Send back but no New fix Ticket", async () => {
    const { w, id } = await world("merge", "cancelled");
    renderAt(w, `/?view=inbox&ticket=${id}`);
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
