// The plain Spec pane, for a Spec that is neither a Requirement nor a Map,
// above the shared Ticket pane: the header (ID, title, submitted time, New
// Prototype), then the Spec's Tickets with their States.
import type { Api } from "../api/api";
import type { NeedsMeReason, SpecDetail } from "../api/types";
import { ago } from "../format";
import { RowLatestEvent } from "../LatestEvent";
import { NewPrototype } from "../requirement/NewRequirement";

export interface SpecPaneProps {
  api: Api;
  spec: SpecDetail;
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function SpecPane({ api, spec, reasons, now, ticket, select }: SpecPaneProps) {
  return (
    <section aria-label="Spec" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Spec · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <div className="spec-actions">
        <NewPrototype api={api} spec={spec.id} onSubmitted={(t) => select(t.id)} />
      </div>
      {spec.tickets.length === 0 ? (
        <p className="muted">No Tickets yet.</p>
      ) : (
        <ul className="impl-list" aria-label="Tickets">
          {spec.tickets.map((t) => {
            const reason = reasons.get(t.id);
            return (
              <li key={t.id}>
                <button className="link" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
                  <span className="tid">{t.id}</span> {t.title}
                </button>{" "}
                <span className={["state", reason && `reason-${reason} loud-state`].filter(Boolean).join(" ")}>{t.state}</span>
                <RowLatestEvent row={t} now={now} />
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
