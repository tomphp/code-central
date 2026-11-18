// The Map view, in the pane a Requirement's view uses and above the
// shared Ticket pane: the header (ID, title, submitted time, the Map's
// count, the Guided toggle, New Prototype, Retro, Write the Spec once the Map is
// finished, its Destination and links to the Requirements made from it),
// then the Map's Tickets as a left-to-right graph with Blockers as edges and
// the fog of what isn't specified yet on the right, beside its collapsible
// Decisions so far and Out of scope.
import { useId, useState } from "react";
import { projectOf } from "../api/api";
import { useApi } from "../live";
import { GUIDED_ON, GUIDED_TAG, type MapView, type SpecDetail, type TicketRow } from "../api/types";
import { Markdown } from "../Markdown";
import { useNeedsMe } from "../needsme/context";
import { specRetroable } from "../edit/editable";
import { NewPrototype, NewRetro, WriteTheSpec } from "../requirement/NewRequirement";
import { RetroList } from "../spec/RetroList";
import type { Link } from "../url";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import { useAct } from "../ticket/useAct";
import { FOG, isFrontier, layoutMap, NODE, nodeProgress } from "./graph";
import { mapCount } from "./mapCount";
import { mapSection } from "./sections";
import { ErrorBanner } from "../controls/ErrorBanner";
import { SpecPaneHeader } from "../controls/SpecPaneHeader";
import { TicketName } from "../ticket/TicketLine";
import { Button } from "../controls/Button";
import { StateChip, stateSlug } from "../controls/StateChip";

export interface MapPaneProps {
  spec: SpecDetail & { map: MapView };
  // archived is whether the Map's Project is Archived, which takes no Retro.
  archived?: boolean;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
  // specLink links another Spec: a Requirement made from the Map.
  specLink(spec: string): Link;
  // written is called with the Requirement Write the Spec made.
  written(spec: string): void;
}

// MapPane is the view: it acts (starts a Ticket, sets Guided) and hands the
// outcome to MapBody, which only renders.
export function MapPane({ spec, ...rest }: MapPaneProps) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const start = (t: TicketRow) => run(() => api.act(projectOf(t.id), t.id, "approve", {}));
  // Guided hands starting the next decision to the Factory: it approves
  // the first frontier Ticket whenever the Map is idle.
  const setGuided = (on: boolean) =>
    run(() => (on ? api.setTags(projectOf(spec.id), spec.id, { [GUIDED_TAG]: GUIDED_ON }) : api.removeTags(projectOf(spec.id), spec.id, [GUIDED_TAG])));
  return <MapBody spec={spec} {...rest} busy={busy} error={error} start={start} setGuided={setGuided} />;
}

export interface MapBodyProps extends MapPaneProps {
  // busy is whether an action is under way, which disables Start and Guided.
  busy: boolean;
  // error is why the last action failed.
  error?: string;
  // start approves a frontier Ticket.
  start(ticket: TicketRow): void;
  setGuided(on: boolean): void;
}

export function MapBody({ spec, archived, now, ticket, select, specLink, written, busy, error, start, setGuided }: MapBodyProps) {
  const { reasonOf } = useNeedsMe();
  const [fogOpen, setFogOpen] = useState(false);
  const { tickets = [], body = "" } = spec;
  const layout = layoutMap(tickets);
  // The arrowhead's ID goes in url(#…), so it keeps only safe characters.
  const arrow = `map-arrow-${useId().replace(/[^\w-]/g, "")}`;
  const destination = mapSection(body, "Destination");
  const fog = mapSection(body, "Not yet specified");

  return (
    <section aria-label="Map" className="req-head map-view">
      <SpecPaneHeader id={spec.id} kind="Map" title={spec.title} now={now} created={spec.created} />
      <p className="map-count">{mapCount(spec.map)}</p>
      <label className="map-guided" title="The Factory starts the next decision whenever the Map is idle">
        <input type="checkbox" checked={spec.map.guided} disabled={busy} onChange={(e) => setGuided(e.target.checked)} /> Guided
      </label>
      <div className="spec-actions">
        <NewPrototype spec={spec.id} onSubmitted={(t) => select(t.id)} />
        {specRetroable(spec, !!archived) && <NewRetro target={spec.id} onSubmitted={(t) => select(t.id)} />}
        {spec.link && <LinkBadge link={spec.link} />}
        <OpenExternal link={spec.link} />
        {spec.map.can_write_spec && (
          <WriteTheSpec map={{ id: spec.id, title: spec.title, destination }} onSubmitted={(r) => written(r.spec.id)} />
        )}
      </div>
      <div aria-label="Destination" className="map-destination">
        {destination ? <Markdown>{destination}</Markdown> : <p className="muted">No Destination yet.</p>}
      </div>
      {spec.map.requirements.length > 0 && (
        <p className="small map-requirements">
          Requirements from this Map:{" "}
          {spec.map.requirements.map((id, i) => (
            <span key={id}>
              {i > 0 && ", "}
              <a className="tid" {...specLink(id)}>
                {id}
              </a>
            </span>
          ))}
        </p>
      )}
      {error && <ErrorBanner>{error}</ErrorBanner>}
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
            {tickets.map((t) => {
              const at = layout.nodes.get(t.id)!;
              const reason = reasonOf(t.id);
              const progress = nodeProgress(t, reason);
              return (
                <foreignObject key={t.id} role="listitem" data-id={t.id} data-progress={progress} x={at.x} y={at.y} width={NODE.width} height={NODE.height}>
                  <div className="map-node-slot">
                  <div
                    className={["map-node", `st-${stateSlug(t.state)}`, `progress-${progress}`, reason && `reason-${reason}`, t.state === "Cancelled" && "cancelled"].filter(Boolean).join(" ")}
                  >
                    <span className="map-node-top">
                      <span className="map-node-type">{t.node_type ?? "ticket"}</span>
                      <StateChip state={t.state} />
                    </span>
                    <button className="link map-node-title" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
                      <TicketName t={t} />
                    </button>
                    {isFrontier(t) && (
                      <Button className="map-start" aria-label={`Start ${t.title}`} disabled={busy} onClick={() => start(t)}>
                        Start
                      </Button>
                    )}
                  </div>
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
          <MapDetails title="Decisions so far" text={mapSection(body, "Decisions so far")} />
          <MapDetails title="Out of scope" text={mapSection(body, "Out of scope")} />
        </div>
      </div>
      <RetroList retros={spec.retros} select={select} />
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
