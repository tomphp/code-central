// The Spec row (in the glossary): the one row the UI shows a Spec in, in
// the Requirements view, Needs Me and By Spec. Its first line is a chevron,
// the ID, a kind badge (none for a Requirement), the title and the
// needs-you dot; then a Requirement's Stage strip or any other Spec's State
// chips; then a one-line headline. The chevron opens it on its Tickets;
// the rest of the row opens the Spec. Whether it is open lives on the page
// only.
import { useEffect, useState, type MouseEvent } from "react";
import { LONG_LIVED_LABEL, STATES, type NeedsMeReason, type StageName, type State, type StateSummary } from "../api/types";
import { REASON_LABEL } from "../format";
import type { ProjectSpec, ProjectTicket } from "../live";
import { mapCount } from "../map/mapCount";
import { headline, progress, stageOf, type Progress } from "../requirement/progress";
import { Strip } from "../requirement/Strip";
import { StateRow, stateSlug, type InboxRowLinkFactory } from "./StateRow";
import { ShortId } from "./ShortId";

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
  link: { href: string; onClick(e: MouseEvent): void; "data-id": string; "aria-current"?: "true" };
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
  const { kind, progress: p, need, needsYou, headline } = describe(spec, all, summary, reasons);
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
            {kind !== "Requirement" && <span className={`kind-badge kind-${stateSlug(kind)}`}>{kind}</span>}
            <span className="row-title">{title}</span>
            {needsYou && <span className={`dot${need ? ` reason-${need}` : ""}`} role="img" aria-label="Needs you" />}
          </span>
          {p ? (
            <Strip p={p} />
          ) : (
            <span className="state-chips">
              {STATES.filter((s) => summary[s]).map((s) => (
                <span key={s} className={`state-chip st-${stateSlug(s)}${s === here ? " here" : ""}`} aria-current={s === here ? "true" : undefined}>
                  {s} {summary[s]}
                </span>
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
            <StateRow key={t.id} t={t} needsYou={reasons.has(t.id)} now={now} link={ticketLink} stage chip={!here} />
          ))}
        </div>
      )}
    </section>
  );
}

// SpecKind is what a Spec row's badge says: nothing for a Requirement.
export type SpecKind = "Requirement" | "Map" | "Long-lived" | "Spec";

export const specKindOf = (spec?: ProjectSpec): SpecKind =>
  spec?.requirement ? "Requirement" : spec?.map ? "Map" : spec?.labels.includes(LONG_LIVED_LABEL) ? "Long-lived" : "Spec";

// describe is what a Spec row says of its Spec, by kind. A Requirement
// has its Stage progress and today's headline, and needs the human when
// any Stage or Build Ticket does. A Map's headline is its decision count,
// and it needs the human when its needs_you count says so. Any other
// Spec's headline is its first need, else whether a Factory is on it, then
// how many of its Tickets are Done; it needs the human when any of its
// Tickets does.
function describe(
  spec: ProjectSpec | undefined,
  all: ProjectTicket[],
  summary: StateSummary,
  reasons: Map<string, NeedsMeReason>,
): { kind: SpecKind; progress?: Progress; need?: NeedsMeReason; needsYou: boolean; headline: string } {
  if (spec?.requirement) {
    const p = progress(spec.requirement, all, reasons);
    return { kind: "Requirement", progress: p, need: p.needs[0]?.reason, needsYou: p.needs.length > 0, headline: headline(spec.finished, p) };
  }
  if (spec?.map) return { kind: "Map", needsYou: spec.map.needs_you > 0, headline: spec.finished ? "Finished" : mapCount(spec.map) };
  const need = firstNeed(all, reasons);
  return { kind: specKindOf(spec), need, needsYou: !!need, headline: spec ? specHeadline(spec.finished, need, all, summary) : "" };
}

function specHeadline(finished: boolean, need: NeedsMeReason | undefined, all: ProjectTicket[], summary: StateSummary): string {
  const done = summary.Done ?? 0;
  const total = STATES.reduce((n, s) => n + (summary[s] ?? 0), 0);
  if (finished) return `Finished · ${done} of ${total} Tickets Done`;
  const doing = need ? REASON_LABEL[need] : all.some((t) => CLAIMED.includes(t.state)) ? "A Factory is on it" : undefined;
  return [doing, total > 0 && `${done} of ${total} Done`].filter(Boolean).join(" · ");
}

// CLAIMED is the States a Ticket holds a live Claim in.
const CLAIMED: State[] = ["In Progress", "Needs Answer", "Answered"];

// firstNeed is the reason of the first of the Tickets that needs the human.
function firstNeed(tickets: ProjectTicket[], reasons: Map<string, NeedsMeReason>): NeedsMeReason | undefined {
  for (const t of tickets) {
    const r = reasons.get(t.id);
    if (r) return r;
  }
  return undefined;
}

function countStates(tickets: ProjectTicket[]): StateSummary {
  const counts: StateSummary = {};
  for (const t of tickets) counts[t.state] = (counts[t.state] ?? 0) + 1;
  return counts;
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
