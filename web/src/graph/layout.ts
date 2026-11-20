// A Ticket graph's layout: the Tickets left to right in layers, laid out
// by dagre with Blockers as edges, and on a Map the fog node at the
// right edge.
import dagre, { type EdgeLabel, type GraphLabel, type NodeLabel } from "@dagrejs/dagre";
import type { TicketRow } from "../api/types";
import { edgesOf } from "./edges";

export const NODE = { width: 196, height: 92 };
export const FOG = { width: 150, height: 44 };
const RANK_GAP = 44;
const MARGIN = 8;

// TopLeft is where a node's top-left corner goes.
export interface TopLeft {
  x: number;
  y: number;
}

export interface GraphLayout {
  width: number;
  height: number;
  nodes: Map<string, TopLeft>;
  // edges run from a Blocker to the Ticket it blocks, as SVG path data.
  edges: { from: string; to: string; d: string }[];
  // fog is where the fog node goes, when the layout makes room for it.
  fog?: TopLeft;
}

// layoutGraph lays the Tickets out left to right, each Blocker left of
// what it blocks; a Blocker outside the set draws no edge. With fog, the
// fog node sits one rank gap right of everything, centred; without it,
// the width ends at the last rank.
export function layoutGraph(tickets: TicketRow[], { fog = false } = {}): GraphLayout {
  const g = new dagre.graphlib.Graph<GraphLabel, NodeLabel, EdgeLabel>();
  g.setGraph({ rankdir: "LR", nodesep: 14, ranksep: RANK_GAP, marginx: MARGIN, marginy: MARGIN });
  g.setDefaultEdgeLabel(() => ({}));
  for (const t of tickets) g.setNode(t.id, { ...NODE });
  for (const e of edgesOf(tickets)) g.setEdge(e.from, e.to);
  dagre.layout(g);

  const nodes = new Map<string, TopLeft>();
  // A lone fog node sits at the margin.
  let right = MARGIN - RANK_GAP;
  let height = (fog ? FOG.height : 0) + 2 * MARGIN;
  for (const t of tickets) {
    // dagre.layout gives every node its centre.
    const { x = 0, y = 0 } = g.node(t.id);
    nodes.set(t.id, { x: x - NODE.width / 2, y: y - NODE.height / 2 });
    right = Math.max(right, x + NODE.width / 2);
    height = Math.max(height, y + NODE.height / 2 + MARGIN);
  }
  const edges = g.edges().map((e) => ({
    from: e.v,
    to: e.w,
    d: (g.edge(e).points ?? []).map((p, i) => `${i ? "L" : "M"}${p.x},${p.y}`).join(" "),
  }));
  if (!fog) return { width: Math.max(right, MARGIN) + MARGIN, height, nodes, edges };
  const at = { x: right + RANK_GAP, y: (height - FOG.height) / 2 };
  return { width: at.x + FOG.width + MARGIN, height, nodes, edges, fog: at };
}
