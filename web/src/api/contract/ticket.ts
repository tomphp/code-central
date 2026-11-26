import { beforeEach, expect, it } from "vitest";
import { ApiError } from "../types";
import type { Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function ticketContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  it("gets one Ticket in full, with its Needs Me reason", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    await d.postRound("acme", id, [{ title: "Which colour?", body: "Red or blue", recommended: "Blue" }]);

    const t = await d.api.getTicket("acme", id);
    expect(t).toMatchObject({ id, title: "Pick a colour", state: "Needs Answer", needs_me_reason: "needs_answer" });
    expect(t.rounds).toHaveLength(1);
    expect(t.rounds![0].questions[0]).toMatchObject({ number: 1, title: "Which colour?", recommended: "Blue", status: "open" });
    expect(Array.isArray(t.labels) && Array.isArray(t.moves) && Array.isArray(t.comments)).toBe(true);
  });

  it("rejects with the one error shape", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Draft");

    const unknown = await d.api.getTicket("nope", "nope-1").catch((e) => e);
    expect(unknown).toBeInstanceOf(ApiError);
    expect(unknown).toMatchObject({ status: 404, code: "unknown_project" });
    expect(unknown.message).not.toBe("");

    const missing = await d.api.getTicket("acme", "acme-99").catch((e) => e);
    expect(missing).toMatchObject({ status: 404, code: "not_found" });

    const same = await d.api.act("acme", id, "move", { to_state: "Draft" }).catch((e) => e);
    expect(same).toMatchObject({ status: 422, code: "invalid", field: "to_state" });
  });

  it("runs a human action on a Ticket, acknowledging it with the fields it changed and the new State", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Draft");
    const before = await d.api.getTicket("acme", id);

    const ack = await d.api.act("acme", id, "move", { to_state: "Ready" });
    wantAcknowledgement(ack, id, "Ready", ["state", "moves"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.state).toBe("Ready");
    expect(ack.updated).toBe(t.updated);
    for (const f of ["title", "description", "comments", "labels"] as const) {
      expect(JSON.stringify(t[f])).toBe(JSON.stringify(before[f]));
      expect(ack.changed).not.toContain(f);
    }
  });

  it("stops a Factory Claim keeping its State, marks it Stopped, and resumes it", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");
    await d.factoryClaim("acme", id, "sess-1");
    expect(await d.api.getTicket("acme", id)).toMatchObject({ state: "In Progress", stoppable: true });

    wantAcknowledgement(await d.api.act("acme", id, "stop", { note: "Pausing" }), id, "In Progress", ["claims", "comments"]);
    const stopped = await d.api.getTicket("acme", id);
    expect(stopped).toMatchObject({ state: "In Progress", needs_me_reason: "stopped", resumable: true });
    expect(stopped.stoppable).toBeFalsy();
    expect(stopped.claims![0]).toMatchObject({ factory_claim: true, stopped: true });
    expect(stopped.claims![0].stops).toHaveLength(1);
    expect(stopped.claims![0].stops![0].resumed).toBeUndefined();
    expect(stopped.comments!.at(-1)).toMatchObject({ actor: "human", body: "Pausing" });

    wantAcknowledgement(await d.api.act("acme", id, "resume", {}), id, "In Progress", ["claims"]);
    const resumed = await d.api.getTicket("acme", id);
    expect(resumed).toMatchObject({ state: "In Progress", stoppable: true });
    expect(resumed.needs_me_reason).toBeUndefined();
    expect(resumed.resumable).toBeFalsy();
    expect(resumed.claims![0].stops).toEqual([{ stopped: expect.any(String), resumed: expect.any(String) }]);
    expect(resumed.claims![0].stopped).toBeFalsy();
  });

  it("refuses to stop what isn't a live Factory Claim, and to resume what isn't Stopped", async () => {
    await d.createProject("acme", "Acme");
    const hand = await d.createTicket("acme", "By hand", "Ready");
    await d.claim("acme", hand, "sess-1");
    expect((await d.api.getTicket("acme", hand)).stoppable).toBeFalsy();
    expect(await d.api.act("acme", hand, "stop", {}).catch((e) => e)).toMatchObject({ status: 409, code: "action_not_allowed" });

    const ready = await d.createTicket("acme", "Ready one", "Ready");
    expect(await d.api.act("acme", ready, "stop", {}).catch((e) => e)).toMatchObject({ status: 409, code: "action_not_allowed" });

    const running = await d.createTicket("acme", "Running", "Ready");
    await d.factoryClaim("acme", running, "sess-2");
    expect(await d.api.act("acme", running, "resume", {}).catch((e) => e)).toMatchObject({ status: 409, code: "action_not_allowed" });
    await d.api.act("acme", running, "stop", {});
    expect(await d.api.act("acme", running, "stop", {}).catch((e) => e)).toMatchObject({ status: 409, code: "action_not_allowed" });
  });

  it("releases a Stopped Claim, which ends the Stopped mark with it", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "A", "Ready");
    await d.factoryClaim("acme", id, "sess-1");
    await d.api.act("acme", id, "stop", {});

    await d.api.act("acme", id, "release", {});
    const t = await d.api.getTicket("acme", id);
    expect(t.state).toBe("Ready");
    expect(t.needs_me_reason).toBeUndefined();
    expect(t.claims![0]).toMatchObject({ ended_by: "release" });
    expect(t.claims![0].stopped).toBeFalsy();
  });
}
