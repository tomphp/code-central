// The Map view's graph: each Ticket's progress and whether it is on the
// frontier, and a left-to-right layered layout of the Tickets, laid out
// by dagre with Blockers as edges, and the fog node at the right edge.
import dagre, { type EdgeLabel, type GraphLabel, type NodeLabel } from "@dagrejs/dagre";
import type { NeedsMeReason, TicketRow } from "../api/types";
import type { SegmentProgress } from "../requirement/progress";

export const NODE = { width: 196, height: 92 };
export const FOG = { width: 150, height: 44 };
const RANK_GAP = 44;
const MARGIN = 8;

// isFrontier reports whether a Map Ticket is on the frontier: Draft and
// not blocked, so the human can Start it.
export const isFrontier = (t: TicketRow) => t.state === "Draft" && !t.blocked;

// nodeProgress is a Map Ticket's colour, in the Stage strip's progress
// states: done once finished, needs while Needs Me lists it (or it waits
// on the human anyway), queued when a Factory can claim it, running
// while one is on it, and waiting when Draft or blocked.
export function nodeProgress(t: TicketRow, reason?: NeedsMeReason): SegmentProgress {
  if (t.state === "Done" || t.state === "Cancelled") return "done";
  if (reason) return "needs";
  switch (t.state) {
    case "Draft":
      return "waiting";
    case "Ready":
      return t.blocked ? "waiting" : "queued";
    case "In Progress":
    case "Answered":
    case "Accepted":
      return "running";
    default:
      return "needs";
  }
}

export interface Placed {
  x: number; // left
  y: number; // top
}

export interface MapLayout {
  width: number;
  height: number;
  nodes: Map<string, Placed>;
  // edges run from a Blocker to the Ticket it blocks, as SVG path data.
  edges: { from: string; to: string; d: string }[];
  fog: Placed;
}

// layoutMap lays the Tickets out left to right, each Blocker left of what
// it blocks; a Blocker off the Map draws no edge. The fog node sits one
// rank gap right of everything, centred.
export function layoutMap(tickets: TicketRow[]): MapLayout {
  const g = new dagre.graphlib.Graph<GraphLabel, NodeLabel, EdgeLabel>();
  g.setGraph({ rankdir: "LR", nodesep: 14, ranksep: RANK_GAP, marginx: MARGIN, marginy: MARGIN });
  g.setDefaultEdgeLabel(() => ({}));
  const ids = new Set(tickets.map((t) => t.id));
  for (const t of tickets) g.setNode(t.id, { ...NODE });
  for (const t of tickets) for (const b of t.blocked_by ?? []) if (ids.has(b)) g.setEdge(b, t.id);
  dagre.layout(g);

  const nodes = new Map<string, Placed>();
  let right = MARGIN - RANK_GAP;
  let height = FOG.height + 2 * MARGIN;
  for (const id of ids) {
    // dagre.layout gives every node its centre.
    const { x = 0, y = 0 } = g.node(id);
    nodes.set(id, { x: x - NODE.width / 2, y: y - NODE.height / 2 });
    right = Math.max(right, x + NODE.width / 2);
    height = Math.max(height, y + NODE.height / 2 + MARGIN);
  }
  const edges = g.edges().map((e) => ({
    from: e.v,
    to: e.w,
    d: (g.edge(e).points ?? []).map((p, i) => `${i ? "L" : "M"}${p.x},${p.y}`).join(" "),
  }));
  const fog = { x: right + RANK_GAP, y: (height - FOG.height) / 2 };
  return { width: fog.x + FOG.width + MARGIN, height, nodes, edges, fog };
}
