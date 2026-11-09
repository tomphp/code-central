// The Requirement pane, above the shared Ticket pane: the header (ID,
// title, submitted time), the clickable Stage strip (its Map segment
// linking to the Map it was made from, if any), what it waits for,
// and under Build its implementation Tickets with their States.
import type { MouseEvent } from "react";
import type { NeedsMeReason, SpecDetail } from "../api/types";
import { ago } from "../format";
import { RowLatestEvent } from "../LatestEvent";
import { progress, waitingNote } from "./progress";
import { Strip } from "./Strip";

export interface RequirementPaneProps {
  spec: SpecDetail & { requirement: NonNullable<SpecDetail["requirement"]> };
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
  // specLink links another Spec, such as the Map it was made from.
  specLink(spec: string): { href: string; onClick(e: MouseEvent): void };
}

export function RequirementPane({ spec, reasons, now, ticket, select, specLink }: RequirementPaneProps) {
  const p = progress(spec.requirement, spec.tickets, reasons);
  const note = waitingNote(p);
  return (
    <section aria-label="Requirement" className="req-head">
      <span className="tid">{spec.id}</span> <span className="muted small">Requirement · submitted {ago(now, spec.created)}</span>
      <h2>{spec.title}</h2>
      <Strip
        p={p}
        current={ticket}
        mapLink={specLink}
        onPick={(s) => {
          const t = s.ticket ?? (p.build.find((t) => reasons.has(t.id)) ?? p.build[0])?.id;
          if (t) select(t);
        }}
      />
      {note && <p className="panel muted waiting">{note}</p>}
      {p.build.length > 0 && (
        <ul className="impl-list" aria-label="Build">
          {p.build.map((t) => {
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
