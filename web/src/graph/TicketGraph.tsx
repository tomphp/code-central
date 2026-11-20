// The Ticket graph: a set of Tickets laid out left to right with Blockers
// as edges, each node coloured by its progress, and choosing one selects
// it. The Map view adds its own extras through the map prop: each node's
// type line, Start on its frontier, and the fog node at the right edge.
import { useId } from "react";
import type { TicketRow } from "../api/types";
import { Button } from "../controls/Button";
import { StateChip, stateSlug } from "../controls/StateChip";
import { useNeedsMe } from "../needsme/context";
import { TicketName } from "../ticket/TicketLine";
import { FOG, layoutGraph, NODE } from "./layout";
import { nodeProgress } from "./progress";

// MapExtras are what the Map view adds to its graph.
export interface MapExtras {
  // frontier reports whether a node gets Start.
  frontier(t: TicketRow): boolean;
  // busy is whether an action is under way, which disables Start.
  busy: boolean;
  // start approves a frontier Ticket.
  start(t: TicketRow): void;
  // fogOpen is whether the fog node's text is showing.
  fogOpen: boolean;
  toggleFog(): void;
}

export interface TicketGraphProps {
  tickets: TicketRow[];
  // label names the graph's list.
  label: string;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
  map?: MapExtras;
}

export function TicketGraph({ tickets, label, ticket, select, map }: TicketGraphProps) {
  const { reasonOf } = useNeedsMe();
  const layout = layoutGraph(tickets, { fog: !!map });
  // The arrowhead's ID goes in url(#…), so it keeps only safe characters.
  const arrow = `ticket-graph-arrow-${useId().replace(/[^\w-]/g, "")}`;

  return (
    <div className="ticket-graph">
      <svg width={layout.width} height={layout.height} role="list" aria-label={label}>
        <defs>
          <marker id={arrow} viewBox="0 0 8 8" refX="8" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
            <path d="M0,0 L8,4 L0,8 z" className="ticket-graph-arrow" />
          </marker>
        </defs>
        <g aria-hidden="true">
          {layout.edges.map((e) => (
            <path key={`${e.from}>${e.to}`} data-edge={`${e.from}>${e.to}`} d={e.d} className="ticket-graph-edge" markerEnd={`url(#${arrow})`} />
          ))}
        </g>
        {tickets.map((t) => {
          const at = layout.nodes.get(t.id)!;
          const reason = reasonOf(t.id);
          const progress = nodeProgress(t, reason);
          return (
            <foreignObject key={t.id} role="listitem" data-id={t.id} data-progress={progress} x={at.x} y={at.y} width={NODE.width} height={NODE.height}>
              <div className="ticket-graph-node-slot">
                <div
                  className={["ticket-graph-node", `st-${stateSlug(t.state)}`, `progress-${progress}`, reason && `reason-${reason}`, t.state === "Cancelled" && "cancelled"]
                    .filter(Boolean)
                    .join(" ")}
                >
                  <span className="ticket-graph-node-top">
                    {map && <span className="map-node-type">{t.node_type ?? "ticket"}</span>}
                    <StateChip state={t.state} />
                  </span>
                  <button className="link ticket-graph-node-title" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
                    <TicketName t={t} />
                  </button>
                  {map?.frontier(t) && (
                    <Button className="map-start" aria-label={`Start ${t.title}`} disabled={map.busy} onClick={() => map.start(t)}>
                      Start
                    </Button>
                  )}
                </div>
              </div>
            </foreignObject>
          );
        })}
        {map && layout.fog && (
          <foreignObject data-fog x={layout.fog.x} y={layout.fog.y} width={FOG.width} height={FOG.height}>
            <button className="map-fog" aria-expanded={map.fogOpen} onClick={map.toggleFog}>
              Not yet specified
            </button>
          </foreignObject>
        )}
      </svg>
    </div>
  );
}
