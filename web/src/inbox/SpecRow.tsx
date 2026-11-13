// The Spec row (in the glossary): the one row the UI shows a Spec in, in
// the Requirements view, Needs Me and By Spec. Its first line is a chevron,
// the ID, a kind badge (none for a Requirement), the title and the
// needs-you dot; then a Requirement's Stage strip or any other Spec's State
// chips; then a one-line headline. The chevron opens it on its Tickets;
// the rest of the row opens the Spec. Whether it is open lives on the page
// only. A Board lane is headed by a Lane header instead.
import { useEffect, useState } from "react";
import { STATES, type NeedsMeReason, type StageName, type State } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { stageOf } from "../requirement/progress";
import { Strip } from "../requirement/Strip";
import { KindBadge } from "../controls/KindBadge";
import { StateChip } from "../controls/StateChip";
import { countStates, describeSpec } from "./describeSpec";
import { ShortId } from "./ShortId";
import { TicketLine, type InboxRowLinkFactory, type RowLink } from "../ticket/TicketLine";

export interface SpecRowProps {
  id: string;
  // spec is missing while the Spec isn't loaded yet; the row then shows
  // its ID as its title.
  spec?: ProjectSpec;
  // all is every one of the Spec's Tickets, finished ones too.
  all: ProjectTicket[];
  // shown is the Tickets the row lists when only some are, in list order;
  // all of them, Stage Tickets first, when left out.
  shown?: ProjectTicket[];
  // here is the State list the row is in, its chip marked; the Tickets it
  // lists then show no State chip of their own.
  here?: State;
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  link: RowLink;
  ticketLink: InboxRowLinkFactory;
  selectedTicket?: string;
  // quietWhenIdle dims the row when it is finished or nothing of it needs
  // the human, as the Requirements view does.
  quietWhenIdle?: boolean;
  // startOpen opens the row when it first shows.
  startOpen?: boolean;
}

export function SpecRow({ id, spec, all, shown, here, reasons, now, link, ticketLink, selectedTicket, quietWhenIdle, startOpen }: SpecRowProps) {
  const listed = shown ?? stagesFirst(all);
  // The row holding the selected Ticket opens on its own, once per
  // selection, so the human can still close it.
  const holdsSelected = !!selectedTicket && listed.some((t) => t.id === selectedTicket);
  const [open, setOpen] = useState(holdsSelected || !!startOpen);
  useEffect(() => {
    if (holdsSelected) setOpen(true);
  }, [selectedTicket, holdsSelected]);

  const title = spec?.title ?? id;
  const summary = spec?.state_summary ?? countStates(all);
  const total = STATES.reduce((n, s) => n + (summary[s] ?? 0), 0);
  const { kind, progress: p, need, needsYou, headline } = describeSpec(spec, all, summary, reasons);
  const quiet = quietWhenIdle && (spec?.finished || !needsYou);

  return (
    <section className={`spec-row${needsYou ? " needs" : ""}${quiet ? " quiet" : ""}`} aria-label={title}>
      <div className="spec-row-head">
        <button className="chev" aria-expanded={open} aria-label={title} onClick={() => setOpen((o) => !o)}>
          {open ? "▾" : "▸"}
        </button>
        <a className="row" {...link}>
          <span className="row-top">
            <ShortId id={id} />
            {kind !== "Requirement" && <KindBadge kind={kind} />}
            <span className="row-title">{title}</span>
            {needsYou && <span className={`dot${need ? ` reason-${need}` : ""}`} role="img" aria-label="Needs you" />}
          </span>
          {p ? (
            <Strip p={p} />
          ) : (
            <span className="state-chips">
              {STATES.filter((s) => summary[s]).map((s) => (
                <StateChip key={s} state={s} here={s === here}>
                  {s} {summary[s]}
                </StateChip>
              ))}
            </span>
          )}
          <span className="spec-row-foot">
            <span className="row-detail">{headline}</span>
            {shown && shown.length < total && (
              <span className="spec-row-count">
                {shown.length} of {total}
              </span>
            )}
          </span>
        </a>
      </div>
      {open && (
        <div className="spec-row-tickets" role="group" aria-label={`Tickets of ${title}`}>
          {listed.map((t) => (
            <TicketLine key={t.id} t={t} link={ticketLink(t.id)} badge={stageOf(t)} chip={!here} needsYou={reasons.has(t.id)} now={now} />
          ))}
        </div>
      )}
    </section>
  );
}

const STAGE_ORDER: StageName[] = ["grilling", "spec", "tickets"];

// stagesFirst is a Spec's Tickets as an open row lists them all: a
// Requirement's Stage Tickets first, in Stage order, then the rest oldest
// first.
export function stagesFirst(tickets: ProjectTicket[]): ProjectTicket[] {
  const rank = (t: ProjectTicket) => {
    const s = stageOf(t);
    return s ? STAGE_ORDER.indexOf(s) : STAGE_ORDER.length;
  };
  return [...tickets].sort((a, b) => rank(a) - rank(b) || a.created.localeCompare(b.created));
}
