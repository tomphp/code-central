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
}
