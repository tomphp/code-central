// hasEdge says whether a set of Tickets has an edge to draw: one in the
// set blocked by another in the set.
import { describe, expect, it } from "vitest";
import { edgesOf, hasEdge } from "./edges";

describe("hasEdge", () => {
  it("is true when a Ticket in the set is blocked by another in the set", () => {
    expect(hasEdge([{ id: "a-1" }, { id: "a-2", blocked_by: ["a-1"] }])).toBe(true);
  });

  it("ignores Blockers outside the set", () => {
    expect(hasEdge([{ id: "a-1", blocked_by: ["b-7"] }, { id: "a-2", blocked_by: ["a-9"] }])).toBe(false);
  });

  it("is false with no Blockers, or no Tickets", () => {
    expect(hasEdge([{ id: "a-1" }, { id: "a-2", blocked_by: [] }])).toBe(false);
    expect(hasEdge([])).toBe(false);
  });
});

describe("edgesOf", () => {
  it("runs each edge from a Blocker in the set to the Ticket it blocks", () => {
    const tickets = [{ id: "a-1" }, { id: "a-2", blocked_by: ["a-1", "b-7"] }, { id: "a-3", blocked_by: ["a-2", "a-1"] }];
    expect(edgesOf(tickets)).toEqual([
      { from: "a-1", to: "a-2" },
      { from: "a-2", to: "a-3" },
      { from: "a-1", to: "a-3" },
    ]);
  });
});
