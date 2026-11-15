import { beforeEach, expect, it } from "vitest";
import type { Driver } from "../driver";

export function needsMeContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  it("reads Needs Me: empty, then grouped with each row's detail", async () => {
    const empty = await d.api.needsMe();
    expect(empty.count).toBe(0);
    expect(empty.counts).toEqual({});
    expect(empty.groups).toEqual([]);

    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    await d.postRound("acme", id, [
      { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
      { title: "Which size?", body: "S or M" },
    ]);

    const v = await d.api.needsMe();
    expect(v.count).toBe(1);
    expect(v.groups.map((g) => g.reason)).toEqual(["needs_answer"]);
    const row = v.groups[0].tickets[0];
    expect(row).toMatchObject({
      project: "acme",
      id,
      title: "Pick a colour",
      state: "Needs Answer",
      reason: "needs_answer",
      round: { number: 1, open: 2, questions: 2 },
    });
    expect(Number.isNaN(Date.parse(row.since))).toBe(false);
  });

  it("counts Needs Me per Project, a Requirement once, and leaves out a Project with nothing", async () => {
    await d.createProject("acme", "Acme");
    await d.createProject("beta", "Beta");
    const one = await d.createTicket("acme", "One", "Ready");
    const two = await d.createTicket("acme", "Two", "Ready");
    await d.postRound("acme", one, [{ title: "Which?", body: "A or B" }]);
    await d.postRound("acme", two, [{ title: "Which?", body: "A or B" }]);
    const req = await d.api.submitRequirement("acme", "Dark mode");
    await d.postRound("acme", req.stages[0].id, [{ title: "Which?", body: "A or B" }]);

    const v = await d.api.needsMe();
    expect(v.counts).toEqual({ acme: 3 });
    expect(v.count).toBe(3);
  });

  it("marks a Needs Me row a Focus Item by its Project's Focus Kinds", async () => {
    await d.createProject("acme", "Acme");
    await d.createProject("beta", "Beta");
    const a = await d.createTicket("acme", "A", "Ready");
    const b = await d.createTicket("beta", "B", "Ready");
    await d.postRound("acme", a, [{ title: "Which?", body: "A or B" }]);
    await d.postRound("beta", b, [{ title: "Which?", body: "A or B" }]);
    const focus = async () => Object.fromEntries((await d.api.needsMe()).groups.flatMap((g) => g.tickets).map((r) => [r.id, r.focus_item]));
    expect(await focus()).toEqual({ [a]: true, [b]: true });

    await d.api.setFocusKinds("acme", ["needs_answer"]);
    expect(await focus()).toEqual({ [a]: false, [b]: true });
  });

  it("gives each found Ticket its needs_me_reason, from its own row or the CI item listing it", async () => {
    await d.createProject("acme", "Acme");
    const asking = await d.createTicket("acme", "Asking", "Ready");
    const quiet = await d.createTicket("acme", "Quiet", "Draft");
    await d.postRound("acme", asking, [{ title: "Which?", body: "A or B" }]);
    const reasons = async () => Object.fromEntries((await d.api.findTickets("acme")).map((r) => [r.id, r.needs_me_reason]));
    expect(await reasons()).toEqual({ [asking]: "needs_answer", [quiet]: undefined });

    const failing = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", failing, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", failing, "accept", {});
    await d.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin", land_per_spec: false });
    await d.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    await d.reportPushedCommit("acme", failing, "abc123");
    await d.reportCIRuns("acme", [
      {
        id: 1,
        attempt: 1,
        workflow: "ci.yml",
        name: "CI",
        branch: "main",
        commit: "abc123",
        status: "completed",
        conclusion: "failure",
        url: "https://github.com/acme/web/actions/runs/1",
        jobs: [],
        ticket: failing,
      },
    ]);
    expect((await reasons())[failing]).toBe("ci_failed");
  });
}
