import { beforeEach, expect, it } from "vitest";
import { ApiError } from "../types";
import { MOVE_EXCEPT } from "../actions/move";
import type { Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function landingContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- Accepted and land (ticket 71) ----

  it("accepts work on a branch into Accepted, then lands it into Done with the commit", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });

    wantAcknowledgement(await d.api.act("acme", id, "accept", {}), id, "Accepted", ["state", "moves"]);
    const a = await d.api.getTicket("acme", id);
    expect(a).toMatchObject({ state: "Accepted", branch: "impl-7", commit: "abc123" });
    expect(a.moves!.at(-1)).toMatchObject({ from: "In Review", to: "Accepted", actor: "human" });

    const blank = await d.api.act("acme", id, "land", { commit: " " }).catch((e) => e);
    expect(blank).toMatchObject({ status: 422, code: "invalid", field: "commit" });

    wantAcknowledgement(await d.api.act("acme", id, "land", { commit: "def456" }), id, "Done", ["state", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.moves!.at(-1)).toMatchObject({ from: "Accepted", to: "Done", actor: "human", via: "merge", commit: "def456" });
  });

  // ---- To merge options (Spec code-central-1067) ----

  it("offers To merge options as send_back and new_fix_ticket accept them", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", id, "accept", {});
    expect(await d.api.getTicket("acme", id)).not.toHaveProperty("to_merge_options");

    await d.api.setLandingMethod("acme", { method: "branch", push: false, remote: "origin" });
    expect((await d.api.getTicket("acme", id)).to_merge_options).toEqual(["send_back"]);

    await d.reportPushRemote("acme", "git@github.com:acme/web.git");
    await d.letGHWrite("acme/web");
    await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin", land_per_spec: false });
    await d.reportPushedCommit("acme", id, "abc123", "impl-7");
    expect((await d.api.getTicket("acme", id)).to_merge_options).toEqual(["send_back"]);

    // Under Land per Spec the Ticket is its Spec's pull-request lead.
    await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin", land_per_spec: true });
    expect((await d.api.getTicket("acme", id)).to_merge_options).toEqual(["new_fix_ticket"]);
    const back = await d.api.act("acme", id, "send_back", { note: "rename it" }).catch((e) => e);
    expect(back).toMatchObject({ status: 409, code: "action_not_allowed" });
    const fix = await d.api.act("acme", id, "new_fix_ticket", { note: "reviewers asked for a rename" });
    const opened = await d.api.getTicket("acme", fix.id);
    expect(opened).toMatchObject({ state: "Ready", spec: { id: (await d.api.getTicket("acme", id)).spec.id } });
    expect(opened).not.toHaveProperty("to_merge_options");
  });

  // ---- Changed Repositories (Spec code-central-848) ----

  it("records the work in every Repository a finish names, and accepts work in another Repository into Accepted", async () => {
    await d.createProject("acme", "Acme");
    await d.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", {
      branch: "impl-7",
      commit: "abc123",
      repositories: [{ name: "api", branch: "impl-7", commit: "def456" }],
    });

    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ branch: "impl-7", commit: "abc123" });
    expect(t.repositories).toEqual([
      { name: "acme", branch: "impl-7", commit: "abc123" },
      { name: "api", branch: "impl-7", commit: "def456" },
    ]);
    const [row] = await d.api.findTickets("acme", { state: ["In Review"] });
    expect(row.repositories).toEqual(["acme", "api"]);

    const other = await d.createTicket("acme", "Login API", "Ready");
    await d.finish("acme", other, "Added the API", { branch: "", commit: "", repositories: [{ name: "api", branch: "impl-8", commit: "fed789" }] });
    wantAcknowledgement(await d.api.act("acme", other, "accept", {}), other, "Accepted", ["state", "moves"]);
  });

  it("lands in each Repository by name, Partly landed and not cancellable until it has landed in every one", async () => {
    await d.createProject("acme", "Acme");
    await d.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", {
      branch: "impl-7",
      commit: "abc123",
      repositories: [{ name: "api", branch: "impl-7", commit: "def456" }],
    });
    await d.api.act("acme", id, "accept", {});

    const unknown = await d.api.act("acme", id, "land", { commit: "fed789", repository: "web" }).catch((e) => e);
    expect(unknown).toMatchObject({ status: 422, code: "invalid", field: "repository" });

    wantAcknowledgement(await d.api.act("acme", id, "land", { commit: "fed789", repository: "api" }), id, "Accepted", ["repositories", "partly_landed"]);
    const partly = await d.api.getTicket("acme", id);
    expect(partly).toMatchObject({ state: "Accepted", partly_landed: true });
    expect(partly.repositories!.map((r) => [r.name, r.landed?.commit])).toEqual([
      ["acme", undefined],
      ["api", "fed789"],
    ]);
    const [row] = await d.api.findTickets("acme", { state: ["Accepted"] });
    expect(row.partly_landed).toBe(true);

    const cancel = await d.api.act("acme", id, "cancel", {}).catch((e) => e);
    expect(cancel).toBeInstanceOf(ApiError);
    expect(cancel).toMatchObject({ status: 409, code: "action_not_allowed" });

    wantAcknowledgement(await d.api.act("acme", id, "land", { commit: "aaa111" }), id, "Done", ["state", "moves", "repositories", "partly_landed"]);
  });

  it("refuses a finish naming a Repository the Project hasn't, keeping the Claim", async () => {
    await d.createProject("acme", "Acme");
    await d.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    const id = await d.createTicket("acme", "Login page", "Ready");
    const refused = await d
      .finish("acme", id, "Done", { branch: "impl-7", commit: "abc123", repositories: [{ name: "web", branch: "impl-7", commit: "def456" }] })
      .catch((e) => e);
    expect(refused).toMatchObject({ status: 422, code: "invalid", field: "repositories[0].name" });
    expect((await d.api.getTicket("acme", id)).state).toBe("In Progress");
  });

  it("cancels an Accepted Ticket", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.api.act("acme", id, "accept", {});

    wantAcknowledgement(await d.api.act("acme", id, "cancel", {}), id, "Cancelled", ["state", "moves"]);
  });

  it("rejects an In Review Ticket back to Ready only with a comment", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page");

    const blank = await d.api.act("acme", id, "reject", { comment: "  " }).catch((e) => e);
    expect(blank).toBeInstanceOf(ApiError);
    expect(blank).toMatchObject({ status: 422, code: "invalid", field: "comment" });
    expect((await d.api.getTicket("acme", id)).state).toBe("In Review");

    wantAcknowledgement(await d.api.act("acme", id, "reject", { comment: "The redirect loops" }), id, "Ready", ["state", "comments", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "The redirect loops" });
    expect((await d.api.needsMe()).count).toBe(0);
  });

  it("refuses accept and reject on a Ticket that isn't In Review", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");

    const a = await d.api.act("acme", id, "accept", {}).catch((e) => e);
    expect(a).toMatchObject({ status: 409, code: "action_not_allowed" });
    const r = await d.api.act("acme", id, "reject", { comment: "no" }).catch((e) => e);
    expect(r).toMatchObject({ status: 409, code: "action_not_allowed" });
  });

  it("implements a finished spec Stage: the Stage is Done, one Ready implementation Ticket is made, and the tickets Stage is Cancelled with it as Replacement", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    const [grilling, spec, tickets] = r.stages.map((s) => s.id);
    await d.api.act("acme", grilling, "move", { to_state: "Done" });
    await d.finish("acme", spec, "Wrote the Spec", { recommendation: "implement" });
    expect((await d.api.getTicket("acme", spec)).recommendation).toBe("implement");

    const ack = await d.api.act("acme", spec, "implement", {});
    wantAcknowledgement(ack, spec, "Done", ["state", "moves"]);
    const done = await d.api.getTicket("acme", spec);
    expect(done.moves!.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human", via: "implement" });

    const s = await d.api.getSpec("acme", r.spec.id);
    const build = s.tickets!.find((t) => t.title === "Implement: Offline mode");
    expect(build).toBeDefined();
    const b = await d.api.getTicket("acme", build!.id);
    expect(b).toMatchObject({ state: "Ready", labels: [], blocked_by: [] });
    expect(b.moves).toMatchObject([{ to: "Ready", actor: "human", via: "implement" }]);
    expect(b.description).toBe(
      `Implement the whole Spec [${r.spec.id}](${r.spec.id}). Its User Stories, Implementation Decisions and Testing Decisions are the acceptance criteria, including writing the glossary terms and ADRs it records. Read them with \`get_spec\` and \`sections\`.\n`,
    );
    const cancelled = await d.api.getTicket("acme", tickets);
    expect(cancelled).toMatchObject({ state: "Cancelled", replacements: [b.id] });
    expect(cancelled.moves!.at(-1)).toMatchObject({ to: "Cancelled", actor: "human", via: "implement" });
    expect(cancelled.comments!.at(-1)).toMatchObject({ actor: "human", body: expect.stringContaining(b.id) });
  });

  it("refuses implement on a Ticket that isn't a spec Stage or isn't In Review, writing nothing", async () => {
    await d.createProject("acme", "Acme");
    const plain = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", plain, "Done it");
    const e = await d.api.act("acme", plain, "implement", {}).catch((e) => e);
    expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(e.message).toContain("not a spec Stage");
    expect((await d.api.getTicket("acme", plain)).state).toBe("In Review");

    const r = await d.api.submitRequirement("acme", "Offline mode");
    const notReview = await d.api.act("acme", r.stages[1].id, "implement", {}).catch((e) => e);
    expect(notReview).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect((await d.api.getSpec("acme", r.spec.id)).tickets).toHaveLength(3);
  });

  // ---- Blockers, move and cancel (ticket 37) ----

  it("removes a Blocker, and rejects one that isn't there", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "A", "Ready");
    const b = await d.createTicket("acme", "B", "Ready");
    await d.addBlocker("acme", b, a);
    expect(await d.api.getTicket("acme", b)).toMatchObject({ blocked_by: [a], blocked: true });

    wantAcknowledgement(await d.api.act("acme", b, "remove_blocker", { blocker: a }), b, "Ready", ["blocked_by", "blockers", "blocked"]);
    expect(await d.api.getTicket("acme", b)).toMatchObject({ blocked_by: [], blockers: [], blocked: false });

    const again = await d.api.act("acme", b, "remove_blocker", { blocker: a }).catch((e) => e);
    expect(again).toMatchObject({ status: 422, code: "invalid", field: "blocker" });
  });

  it("cancels as the human with no Replacement, leaving its dependant blocked by a Cancelled Ticket", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "A", "Ready");
    const b = await d.createTicket("acme", "B", "Ready");
    await d.addBlocker("acme", b, a);

    wantAcknowledgement(await d.api.act("acme", a, "cancel", {}), a, "Cancelled", ["state", "moves"]);
    const t = await d.api.getTicket("acme", a);
    expect(t).toMatchObject({ state: "Cancelled", replacements: [] });
    expect(t.moves!.at(-1)).toMatchObject({ from: "Ready", to: "Cancelled", actor: "human" });
    const dep = await d.api.getTicket("acme", b);
    expect(dep).toMatchObject({ blocked: true, blockers: [{ id: a, title: "A", state: "Cancelled", cleared: false }] });
  });

  it("moves to any State but Needs Answer and Failed", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");
    // MOVE_EXCEPT is what the UI leaves out of its Move to list.
    expect(MOVE_EXCEPT).toHaveLength(2);
    for (const to of MOVE_EXCEPT) {
      const err = await d.api.act("acme", id, "move", { to_state: to }).catch((e) => e);
      expect(err).toMatchObject({ status: 422, code: "invalid", field: "to_state" });
    }
    wantAcknowledgement(await d.api.act("acme", id, "move", { to_state: "Done" }), id, "Done", ["state", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.moves!.at(-1)).toMatchObject({ from: "Ready", to: "Done", actor: "human" });
  });

  // ---- release (ticket 35) ----
  it("releases a Claim back to Ready, ending it, with the note as a comment", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");
    await d.claim("acme", id, "sess-1");

    wantAcknowledgement(await d.api.act("acme", id, "release", { note: "Session died" }), id, "Ready", ["state", "claims", "comments", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.claims).toHaveLength(1);
    expect(t.claims![0]).toMatchObject({ session_id: "sess-1", ended_by: "release" });
    expect(t.claims![0].ended).toBeTruthy();
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "Session died" });

    const again = await d.api.act("acme", id, "release", {}).catch((e) => e);
    expect(again).toMatchObject({ status: 409, code: "action_not_allowed" });
  });

  it("releases without a note, adding no comment", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");
    await d.claim("acme", id, "sess-1");

    const ack = await d.api.act("acme", id, "release", {});
    wantAcknowledgement(ack, id, "Ready", ["state", "claims", "moves"]);
    expect(ack.changed).not.toContain("comments");
    expect((await d.api.getTicket("acme", id)).comments).toEqual([]);
  });
}
