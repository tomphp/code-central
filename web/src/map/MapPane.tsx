// The Map view, in the pane a Requirement's view uses and above the
// shared Ticket pane: the header (ID, title, submitted time, the Map's
// count, the Guided toggle, New Prototype, Retro, Write the Spec once the Map is
// finished, its Destination and links to the Requirements made from it),
// then the Map's Ticket graph, with Start on its frontier and the fog of
// what isn't specified yet on the right, beside its collapsible Decisions
// so far and Out of scope.
import { useState } from "react";
import { projectOf } from "../api/api";
import { useApi } from "../live";
import { GUIDED_ON, GUIDED_TAG, type MapView, type SpecDetail, type TicketRow } from "../api/types";
import { Markdown } from "../Markdown";
import { NewPrototype, NewRetro, WriteTheSpec } from "../requirement/NewRequirement";
import { RetroList } from "../spec/RetroList";
import type { Link } from "../url";
import { LinkBadge } from "../ticket/LinkBadge";
import { OpenExternal } from "../ticket/OpenExternal";
import { useAct } from "../ticket/useAct";
import { TicketGraph } from "../graph/TicketGraph";
import { isFrontier } from "./frontier";
import { mapCount } from "./mapCount";
import { mapSection } from "./sections";
import { ErrorBanner } from "../controls/ErrorBanner";
import { SpecPaneHeader } from "../controls/SpecPaneHeader";

export interface MapPaneProps {
  spec: SpecDetail & { map: MapView };
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

export function MapBody({ spec, now, ticket, select, specLink, written, busy, error, start, setGuided }: MapBodyProps) {
  const [fogOpen, setFogOpen] = useState(false);
  const { tickets = [], body = "" } = spec;
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
        {spec.retroable && <NewRetro target={spec.id} onSubmitted={(t) => select(t.id)} />}
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
        <TicketGraph
          tickets={tickets}
          label="Map Tickets"
          ticket={ticket}
          select={select}
          map={{ frontier: isFrontier, busy, start, fogOpen, toggleFog: () => setFogOpen(!fogOpen) }}
        />
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
