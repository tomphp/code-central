import { beforeEach, expect, it } from "vitest";
import { ApiError } from "../types";
import { sampleReport, type Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function reviewContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- Failed and In Review (ticket 36) ----

  it("shows a Failed Ticket's Failure Reports and lists it in Needs Me", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Fix login", "Ready");
    await d.reportFailure("acme", id, sampleReport);

    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ state: "Failed", needs_me_reason: "failed" });
    expect(t.failure_reports).toHaveLength(1);
    expect(t.failure_reports![0]).toMatchObject({ number: 1, claim: 1, session_id: "contract-session", ...sampleReport });
    const v = await d.api.needsMe();
    expect(v.groups[0]).toMatchObject({ reason: "failed", tickets: [{ id, failure: { headline: "Tests fail on CI", reports: 1 } }] });
  });

  it("sends a Failed Ticket back to Ready, with the note as a human comment", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Fix login", "Ready");
    await d.reportFailure("acme", id, sampleReport);

    wantAcknowledgement(await d.api.act("acme", id, "send_back", { note: "Fixture fixed on main" }), id, "Ready", ["state", "comments", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "Fixture fixed on main" });
    expect(t.moves!.at(-1)).toMatchObject({ from: "Failed", to: "Ready", actor: "human" });
    expect((await d.api.needsMe()).count).toBe(0);
  });

  it("sends a Failed Ticket back with no note and adds no comment", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Fix login", "Ready");
    await d.reportFailure("acme", id, sampleReport);
    const before = (await d.api.getTicket("acme", id)).comments!.length;

    const ack = await d.api.act("acme", id, "send_back", {});
    wantAcknowledgement(ack, id, "Ready", ["state", "moves"]);
    expect(ack.changed).not.toContain("comments");
    expect((await d.api.getTicket("acme", id)).comments).toHaveLength(before);
  });

  it("moves a Failed Ticket to Draft", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Fix login", "Ready");
    await d.reportFailure("acme", id, sampleReport);

    wantAcknowledgement(await d.api.act("acme", id, "to_draft", {}), id, "Draft", ["state", "moves"]);
    expect((await d.api.needsMe()).count).toBe(0);
  });

  it("cancels a Failed Ticket", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Fix login", "Ready");
    await d.reportFailure("acme", id, sampleReport);

    wantAcknowledgement(await d.api.act("acme", id, "cancel", {}), id, "Cancelled", ["state", "moves"]);
    expect(await d.api.getTicket("acme", id)).toMatchObject({ state: "Cancelled", replacements: [] });
    expect((await d.api.needsMe()).count).toBe(0);
  });

  it("refuses send_back and to_draft on a Ticket that isn't Failed", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");

    for (const action of ["send_back", "to_draft"] as const) {
      const e = await d.api.act("acme", id, action, {}).catch((e) => e);
      expect(e).toBeInstanceOf(ApiError);
      expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
    }
  });

  it("shows an In Review Ticket with its finish summary", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page\nBranch impl/7; check the redirect");

    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ state: "In Review", needs_me_reason: "in_review" });
    const entered = t.moves!.at(-1)!;
    expect(entered).toMatchObject({ from: "In Progress", to: "In Review", actor: "agent" });
    expect(t.comments!.at(-1)).toMatchObject({
      actor: "agent",
      time: entered.time,
      body: "Added the login page\nBranch impl/7; check the redirect",
    });
    expect((await d.api.needsMe()).groups[0]).toMatchObject({
      reason: "in_review",
      tickets: [{ id, summary: "Added the login page" }],
    });
  });

  it("accepts an In Review Ticket into Done", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    await d.finish("acme", id, "Added the login page");

    wantAcknowledgement(await d.api.act("acme", id, "accept", {}), id, "Done", ["state", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.moves!.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human" });
    expect((await d.api.needsMe()).count).toBe(0);
  });
}
