// The Map view, in the pane a Requirement's view uses and above the
// shared Ticket pane: the header (ID, title, submitted time, the Map's
// count, New Prototype and its Destination), then the Map's Tickets as a left-to-right
// graph with Blockers as edges and the fog of what isn't specified yet on
// the right, beside its collapsible Decisions so far and Out of scope.
import { useId, useState } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { MapView, NeedsMeReason, SpecDetail, TicketRow } from "../api/types";
import { ago } from "../format";
import { Markdown } from "../Markdown";
import { NewPrototype } from "../requirement/NewRequirement";
import { useAct } from "../ticket/useAct";
import { FOG, isFrontier, layoutMap, NODE, nodeProgress } from "./graph";
import { mapCount } from "./MapRow";
import { mapSection } from "./sections";

export interface MapPaneProps {
  api: Api;
  spec: SpecDetail & { map: MapView };
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function MapPane({ api, spec, reasons, now, ticket, select }: MapPaneProps) {
  const [fogOpen, setFogOpen] = useState(false);
  const { busy, error, run } = useAct();
  const layout = layoutMap(spec.tickets);
  // The arrowhead's ID goes in url(#…), so it keeps only safe characters.
  const arrow = `map-arrow-${useId().replace(/[^\w-]/g, "")}`;
  const destination = mapSection(spec.body, "Destination");
  const fog = mapSection(spec.body, "Not yet specified");
  const start = (t: TicketRow) => run(() => api.act(projectOf(t.id), t.id, "approve", {}));

  return (
    <section aria-label="Map" className="req-head map-view">
      <span className="tid">{spec.id}</span> <span className="muted small">Map · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <p className="map-count">{mapCount(spec.map)}</p>
      <div className="spec-actions">
        <NewPrototype api={api} spec={spec.id} onSubmitted={(t) => select(t.id)} />
      </div>
      <div aria-label="Destination" className="map-destination">
        {destination ? <Markdown>{destination}</Markdown> : <p className="muted">No Destination yet.</p>}
      </div>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
      <div className="map-body">
        <div className="map-graph">
          <svg width={layout.width} height={layout.height} role="list" aria-label="Map Tickets">
            <defs>
              <marker id={arrow} viewBox="0 0 8 8" refX="8" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                <path d="M0,0 L8,4 L0,8 z" className="map-arrow" />
              </marker>
            </defs>
            <g aria-hidden="true">
              {layout.edges.map((e) => (
                <path key={`${e.from}>${e.to}`} data-edge={`${e.from}>${e.to}`} d={e.d} className="map-edge" markerEnd={`url(#${arrow})`} />
              ))}
            </g>
            {spec.tickets.map((t) => {
              const at = layout.nodes.get(t.id)!;
              const reason = reasons.get(t.id);
              const progress = nodeProgress(t, reason);
              return (
                <foreignObject key={t.id} role="listitem" data-id={t.id} data-progress={progress} x={at.x} y={at.y} width={NODE.width} height={NODE.height}>
                  <div
                    className={["map-node", `progress-${progress}`, reason && `reason-${reason}`, t.state === "Cancelled" && "cancelled"].filter(Boolean).join(" ")}
                  >
                    <span className="map-node-top">
                      <span className="map-node-type">{t.node_type ?? "ticket"}</span>
                      <span className="state">{t.state}</span>
                    </span>
                    <button className="link map-node-title" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
                      <span className="tid">{t.id}</span> {t.title}
                    </button>
                    {isFrontier(t) && (
                      <button className="btn map-start" aria-label={`Start ${t.title}`} disabled={busy} onClick={() => start(t)}>
                        Start
                      </button>
                    )}
                  </div>
                </foreignObject>
              );
            })}
            <foreignObject data-fog x={layout.fog.x} y={layout.fog.y} width={FOG.width} height={FOG.height}>
              <button className="map-fog" aria-expanded={fogOpen} onClick={() => setFogOpen(!fogOpen)}>
                Not yet specified
              </button>
            </foreignObject>
          </svg>
        </div>
        <div className="map-side">
          <MapDetails title="Decisions so far" text={mapSection(spec.body, "Decisions so far")} />
          <MapDetails title="Out of scope" text={mapSection(spec.body, "Out of scope")} />
        </div>
      </div>
      {fogOpen && (
        <section aria-label="Not yet specified" className="panel map-fog-text">
          <h3>Not yet specified</h3>
          {fog ? <Markdown>{fog}</Markdown> : <p className="muted">Nothing written yet.</p>}
        </section>
      )}
    </section>
  );
}

function MapDetails({ title, text }: { title: string; text: string }) {
  return (
    <details open className="map-details">
      <summary>{title}</summary>
      {text ? <Markdown>{text}</Markdown> : <p className="muted">None yet.</p>}
    </details>
  );
}
