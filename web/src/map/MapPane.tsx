// The Map pane, above the shared Ticket pane: the header (ID, title,
// submitted time), the Map's count, and its Tickets with their type and
// State.
import type { MapView, NeedsMeReason, SpecDetail } from "../api/types";
import { ago } from "../format";
import { RowLatestEvent } from "../LatestEvent";
import { mapCount } from "./MapRow";

export interface MapPaneProps {
  spec: SpecDetail & { map: MapView };
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function MapPane({ spec, reasons, now, ticket, select }: MapPaneProps) {
  return (
    <section aria-label="Map" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Map · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <p className="map-count">{mapCount(spec.map)}</p>
      <ul className="impl-list" aria-label="Map Tickets">
        {spec.tickets.map((t) => {
          const reason = reasons.get(t.id);
          return (
            <li key={t.id}>
              <button className="link" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
                <span className="tid">{t.id}</span> {t.title}
              </button>{" "}
              {t.node_type && <span className="muted">{t.node_type}</span>}{" "}
              <span className={["state", reason && `reason-${reason} loud-state`].filter(Boolean).join(" ")}>{t.state}</span>
              <RowLatestEvent row={t} now={now} />
            </li>
          );
        })}
      </ul>
    </section>
  );
}
