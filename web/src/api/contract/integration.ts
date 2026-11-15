import { beforeEach, expect, it } from "vitest";
import { ApiError, TRIAGE_ACTIONS } from "../types";
import type { Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function integrationContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- the Integration contract (ticket 42) ----
  it("edits a Ticket's title and description, recording one edit by human with only what changed", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Pick a colour", "Draft");

    const one = await d.api.act("acme", id, "edit_ticket", { description: "Blue." });
    wantAcknowledgement(one, id, "Draft", ["description", "edits"]);
    expect(one.changed).not.toContain("title");
    expect(one.edit).toBe(1);
    const both = await d.api.act("acme", id, "edit_ticket", { title: "Pick a shade", description: "Navy." });
    wantAcknowledgement(both, id, "Draft", ["title", "description", "edits"]);

    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ title: "Pick a shade", description: "Navy." });
    expect(t.edits!.map((e) => [e.actor, e.previous])).toEqual([
      ["human", { description: "" }],
      ["human", { title: "Pick a colour", description: "Blue." }],
    ]);
  });

  it("writes no edit for a Ticket change that changes nothing, and refuses an empty title", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Pick a colour", "Draft");
    const same = await d.api.act("acme", id, "edit_ticket", { title: "Pick a colour", description: "" });
    expect(same.changed).toEqual([]);
    expect(same.edit).toBeUndefined();
    expect((await d.api.getTicket("acme", id)).edits).toEqual([]);

    const empty = await d.api.act("acme", id, "edit_ticket", { title: "  " }).catch((e) => e);
    expect(empty).toBeInstanceOf(ApiError);
    expect(empty).toMatchObject({ status: 422, code: "invalid", field: "title" });
    expect((await d.api.getTicket("acme", id)).title).toBe("Pick a colour");
  });

  it("still edits a Ready Ticket, and refuses a definition-owned linked Ticket until it is unlinked", async () => {
    await d.createProject("acme", "Acme");
    const ready = await d.createTicket("acme", "Ready one", "Ready");
    wantAcknowledgement(await d.api.act("acme", ready, "edit_ticket", { title: "Ready two" }), ready, "Ready", ["title", "edits"]);

    const gh = { integration: "github", id: "acme/web#9", url: "https://github.com/acme/web/issues/9" };
    const linked = await d.linkTicket("acme", "Issue", gh, "open");
    const refused = await d.api.act("acme", linked, "edit_ticket", { title: "Mine" }).catch((e) => e);
    expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect((await d.api.getTicket("acme", linked)).title).toBe("Issue");

    await d.api.unlink("acme", linked);
    wantAcknowledgement(await d.api.act("acme", linked, "edit_ticket", { title: "Mine" }), linked, "Draft", ["title", "edits"]);
  });

  it("edits a Spec's title and body, recording one edit by human with only what changed", async () => {
    await d.createProject("acme", "Acme");
    const { spec } = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    const before = await d.api.getSpec("acme", spec.id);

    const one = await d.api.editSpec("acme", spec.id, { body: "New body." });
    expect(one).toMatchObject({ id: spec.id, edit: before.edits!.length + 1 });
    expect(one.changed).toEqual(expect.arrayContaining(["body", "edits"]));
    expect(one.changed).not.toContain("title");
    const both = await d.api.editSpec("acme", spec.id, { title: "Flight mode", body: "Newer body." });
    expect(both.changed).toEqual(expect.arrayContaining(["title", "body", "edits"]));

    const s = await d.api.getSpec("acme", spec.id);
    expect(s).toMatchObject({ title: "Flight mode", body: "Newer body." });
    expect(s.edits!.slice(before.edits!.length).map((e) => [e.actor, Object.keys(e.previous).sort()])).toEqual([
      ["human", ["body"]],
      ["human", ["body", "title"]],
    ]);
    expect(s.edits!.at(-1)!.previous.title).toBe("Offline mode");
  });

  it("writes no edit for a Spec change that changes nothing, and refuses an empty title", async () => {
    await d.createProject("acme", "Acme");
    const { spec } = await d.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    const s = await d.api.getSpec("acme", spec.id);
    const same = await d.api.editSpec("acme", spec.id, { title: s.title, body: s.body });
    expect(same.changed).toEqual([]);
    expect(same.edit).toBeUndefined();
    expect((await d.api.getSpec("acme", spec.id)).edits).toEqual(s.edits);

    const empty = await d.api.editSpec("acme", spec.id, { title: "" }).catch((e) => e);
    expect(empty).toMatchObject({ status: 422, code: "invalid", field: "title" });
  });

  it("still edits a Finished Spec, and refuses an edit in an Archived Project", async () => {
    await d.createProject("acme", "Acme");
    const { spec, stages } = await d.api.submitRequirement("acme", "Offline mode");
    for (const t of stages) await d.api.act("acme", t.id, "cancel", {});
    expect((await d.api.getSpec("acme", spec.id)).finished).toBe(true);
    const ack = await d.api.editSpec("acme", spec.id, { body: "Still editable." });
    expect(ack.changed).toContain("body");

    const t = await d.createTicket("acme", "Draft one", "Draft");
    await d.api.act("acme", t, "cancel", {});
    await d.api.archiveProject("acme");
    const spec1 = await d.api.editSpec("acme", spec.id, { body: "No." }).catch((e) => e);
    expect(spec1).toMatchObject({ status: 409, code: "action_not_allowed" });
    const ticket1 = await d.api.act("acme", t, "edit_ticket", { title: "No" }).catch((e) => e);
    expect(ticket1).toMatchObject({ status: 409, code: "action_not_allowed" });
  });

  it("reads a linked Ticket closed externally, and unlinks it", async () => {
    await d.createProject("acme", "Acme");
    const gh = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };
    const id = await d.linkTicket("acme", "Issue", gh, "closed");
    const link = { ...gh, external_state: "closed", detached: false, outbound: false };

    expect(await d.api.getTicket("acme", id)).toMatchObject({ link, needs_me_reason: "closed_externally" });
    const nm = await d.api.needsMe();
    expect(nm.groups.map((g) => g.reason)).toEqual(["closed_externally"]);
    expect(nm.groups[0].tickets[0]).toMatchObject({ id, reason: "closed_externally", link });
    expect((await d.api.findTickets("acme")).find((r) => r.id === id)?.link).toEqual(link);

    const row = await d.api.unlink("acme", id);
    expect(row).toMatchObject({ kind: "ticket", id, link: { ...link, detached: true } });
    const t = await d.api.getTicket("acme", id);
    expect(t.link?.detached).toBe(true);
    expect(t.needs_me_reason).toBeUndefined();
    expect((await d.api.needsMe()).count).toBe(0);

    const plain = await d.createTicket("acme", "Local", "Draft");
    const err = await d.api.unlink("acme", plain).catch((e) => e);
    expect(err).toMatchObject({ status: 422, code: "invalid", field: "target" });
  });

  // ---- New issue and Keep as Draft (code-central-287) ----

  // incomingIssue turns GitHub Issues on for a new Project acme, from
  // acme/web, with New issues need me as needMe, and brings acme/web's
  // issue n in as title; it returns its Ticket and its issue's link.
  async function incomingIssue(needMe: boolean, n: number, title: string) {
    await d.createProject("acme", "Acme");
    await d.letGHTriage("acme/web");
    const on = await d.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: needMe });
    const gh = { integration: "github", id: `acme/web#${n}`, url: `https://github.com/acme/web/issues/${n}` };
    return { id: await d.incomingIssue("acme", on.github_issues.spec!, title, gh), gh };
  }

  it("shows a New issue in Needs Me until the human keeps it as a Draft", async () => {
    const { id, gh } = await incomingIssue(true, 7, "Login is broken");
    const link = { ...gh, external_state: "open", detached: false, outbound: false };

    expect(await d.api.getTicket("acme", id)).toMatchObject({ state: "Draft", link, awaiting_triage: true, triage_actions: TRIAGE_ACTIONS, needs_me_reason: "new_issue" });
    const nm = await d.api.needsMe();
    expect(nm.groups.map((g) => g.reason)).toEqual(["new_issue"]);
    expect(nm.groups[0].tickets[0]).toMatchObject({ id, reason: "new_issue", link });

    wantAcknowledgement(await d.api.act("acme", id, "keep_as_draft", {}), id, "Draft", ["needs_me_reason", "awaiting_triage"]);
    const kept = await d.api.getTicket("acme", id);
    expect(kept).toMatchObject({ state: "Draft", triage_actions: TRIAGE_ACTIONS });
    expect(kept.awaiting_triage).toBeUndefined();
    expect(kept.needs_me_reason).toBeUndefined();
    expect((await d.api.needsMe()).count).toBe(0);

    const plain = await d.createTicket("acme", "Local", "Draft");
    const refused = await d.api.act("acme", plain, "keep_as_draft", {}).catch((e) => e);
    expect(refused).toBeInstanceOf(ApiError);
    expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(refused.message).toContain("not an Incoming Issue");
  });

  it("offers the triage actions on an Incoming Issue that came in with New issues need me off", async () => {
    const { id } = await incomingIssue(false, 8, "Typo");
    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ triage_actions: TRIAGE_ACTIONS });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.needs_me_reason).toBeUndefined();
    expect((await d.api.needsMe()).count).toBe(0);

    // Unlinked, it is no longer an Incoming Issue.
    await d.api.unlink("acme", id);
    expect((await d.api.getTicket("acme", id)).triage_actions).toBeUndefined();
  });

  // ---- Won't fix (code-central-289) ----
  it("Cancels an Incoming Issue on Won't fix, with the reason as its comment", async () => {
    const { id } = await incomingIssue(true, 7, "Login is broken");

    wantAcknowledgement(await d.api.act("acme", id, "wont_fix", { reason: "Works as intended." }), id, "Cancelled", ["state", "needs_me_reason", "awaiting_triage", "triage_actions"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "Works as intended." });
    expect(t.moves!.at(-1)).toMatchObject({ to: "Cancelled", via: "wont_fix" });
    expect(t.link).toMatchObject({ detached: false });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.triage_actions).toBeUndefined();
    expect((await d.api.needsMe()).count).toBe(0);

    const plain = await d.createTicket("acme", "Local", "Draft");
    const refused = await d.api.act("acme", plain, "wont_fix", {}).catch((e) => e);
    expect(refused).toBeInstanceOf(ApiError);
    expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(refused.message).toContain("not an Incoming Issue");

    // Unlinked, an Incoming Issue is refused, naming its Link.
    const spec = (await d.api.getTicket("acme", id)).spec.id;
    const other = await d.incomingIssue("acme", spec, "Typo", { integration: "github", id: "acme/web#8", url: "https://github.com/acme/web/issues/8" });
    await d.api.unlink("acme", other);
    const unlinked = await d.api.act("acme", other, "wont_fix", {}).catch((e) => e);
    expect(unlinked).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(unlinked.message).toContain(`${other} was unlinked from github acme/web#8`);
  });

  // ---- To Requirement (code-central-290) ----
  it("turns an Incoming Issue into a Requirement whose Spec has an outbound Link", async () => {
    const { id, gh } = await incomingIssue(true, 7, "Login is broken");

    const ack = await d.api.act("acme", id, "to_requirement", { context: "  Seen on every login.  " });
    wantAcknowledgement(ack, id, "Cancelled", ["state", "replacements", "comments", "moves", "needs_me_reason", "link", "awaiting_triage", "triage_actions"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "Seen on every login." });
    expect(t.moves!.at(-1)).toMatchObject({ to: "Cancelled", via: "to_requirement" });
    expect(t.link).toMatchObject({ ...gh, detached: true, outbound: false });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.triage_actions).toBeUndefined();
    expect((await d.api.needsMe()).count).toBe(0);

    const grilling = await d.api.getTicket("acme", t.replacements[0]);
    expect(grilling.labels).toEqual(["stage:grilling"]);
    const sp = await d.api.getSpec("acme", grilling.spec.id);
    expect(sp).toMatchObject({ title: "Login is broken", body: "# Login is broken\n\n## Context\n\nSeen on every login.\n" });
    expect(sp.link).toEqual({ ...gh, external_state: "open", detached: false, outbound: true });
    expect(sp.requirement?.stages.map((s) => s.ticket)).toEqual([grilling.id, expect.any(String), expect.any(String)]);

    const plain = await d.createTicket("acme", "Local", "Draft");
    const refused = await d.api.act("acme", plain, "to_requirement", {}).catch((e) => e);
    expect(refused).toBeInstanceOf(ApiError);
    expect(refused).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(refused.message).toContain("not an Incoming Issue");
  });
}
