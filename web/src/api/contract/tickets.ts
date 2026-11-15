import { beforeEach, expect, it } from "vitest";
import type { State } from "../types";
import type { Driver } from "../driver";

export function ticketsContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  it("finds a Project's Tickets, optionally by State", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "A", "Draft");
    const b = await d.createTicket("acme", "B", "Ready");

    const all = await d.api.findTickets("acme");
    expect(all.map((r) => [r.id, r.title, r.state]).sort()).toEqual([
      [a, "A", "Draft"],
      [b, "B", "Ready"],
    ]);
    const ready = await d.api.findTickets("acme", { state: ["Ready"] });
    expect(ready.map((r) => r.id)).toEqual([b]);
    const either = await d.api.findTickets("acme", { state: ["Ready", "Draft"] as State[] });
    expect(either).toHaveLength(2);
  });

  it("carries created on a row, never decreasing for Tickets made in order", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "A", "Draft");
    const b = await d.createTicket("acme", "B", "Draft");

    const rows = await d.api.findTickets("acme");
    const created = (id: string) => rows.find((r) => r.id === id)!.created;
    expect(Number.isNaN(Date.parse(created(a)))).toBe(false);
    // created has one-second resolution, so Tickets made in the same second tie.
    expect(Date.parse(created(b))).toBeGreaterThanOrEqual(Date.parse(created(a)));
  });

  it("leaves finished Tickets out of findTickets unless the filter names them", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "A", "Ready");
    const b = await d.createTicket("acme", "B", "Ready");
    await d.api.act("acme", b, "move", { to_state: "Done" });

    expect((await d.api.findTickets("acme")).map((r) => r.id)).toEqual([a]);
    expect((await d.api.findTickets("acme", { state: ["Done", "Cancelled"] })).map((r) => [r.id, r.state])).toEqual([[b, "Done"]]);
  });
}
