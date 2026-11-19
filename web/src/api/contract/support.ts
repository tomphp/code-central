import { expect } from "vitest";
import type { State, TicketAcknowledgement, UsageSession } from "../types";

// wantAcknowledgement checks a human action's result is its Ticket's
// acknowledgement (ADR 0009): the ID, updated time, the names of the
// fields it changed (changed among them) and the State, and no record. An
// operation's next_step may ride along.
export function wantAcknowledgement(ack: TicketAcknowledgement, id: string, state: State, changed: string[] = []) {
  expect(Object.keys(ack).filter((k) => k !== "edit" && k !== "next_step").sort()).toEqual(["changed", "id", "state", "updated"]);
  expect(ack).toMatchObject({ id, state });
  expect(Number.isNaN(Date.parse(ack.updated))).toBe(false);
  expect(ack.changed).toEqual(expect.arrayContaining(changed));
  expect(ack.changed).not.toContain("updated");
}

// withoutHost is a Watcher's status without the host of the service
// holding the Project's Watcher Lease: a real service takes the Lease
// once CI Tracking is on and names its own machine, which the fake's
// world leaves out until a test gives it a Lease (HoldWatcherLease).
export function withoutHost<W extends { host?: string }>(w: W): Omit<W, "host"> {
  const rest = { ...w };
  delete rest.host;
  return rest;
}

// startedAt is a Usage session with its start, and its Subagents', in
// milliseconds, so the service's and the fake's times compare.
export function startedAt(s: UsageSession): object {
  return { ...s, started: Date.parse(s.started), subagents: s.subagents.map(startedAt) };
}
