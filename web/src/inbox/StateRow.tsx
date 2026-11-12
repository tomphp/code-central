// A State list's two-line Ticket row, shared by the By Ticket grouping and
// the Spec row: the latest Session Event under the title, and a dot when the
// Ticket needs the human.
import type { MouseEvent } from "react";
import { RowLatestEvent } from "../LatestEvent";
import type { ProjectTicket } from "../live";
import { stageOf } from "../requirement/progress";
import { LinkBadge } from "../ticket/LinkBadge";
import { WaitingBadge } from "../ticket/WaitingBadge";
import { ShortId } from "./ShortId";

export type InboxRowLinkFactory = (id: string) => {
  href: string;
  onClick(e: MouseEvent): void;
  "data-id": string;
  "aria-current"?: "true";
};

// stateSlug is a State, or a Spec's kind, as a class name: "In Progress"
// is in-progress.
export const stateSlug = (s: string) => s.toLowerCase().replace(/\s+/g, "-");

// stage shows the Ticket's Stage as a badge, for a Stage Ticket sitting
// among its Requirement's Tickets; chip shows its State, in a list of
// several States.
export function StateRow({ t, needsYou, now, link, stage, chip }: { t: ProjectTicket; needsYou: boolean; now: Date; link: InboxRowLinkFactory; stage?: boolean; chip?: boolean }) {
  const name = stage ? stageOf(t) : undefined;
  return (
    <a className="row" {...link(t.id)}>
      <span className="row-top">
        <ShortId id={t.id} />
        {name && <span className="stage-badge">{name}</span>}
        <span className="row-title">{t.title}</span>
        {chip && <span className={`state-chip st-${stateSlug(t.state)}`}>{t.state}</span>}
        {t.link && <LinkBadge link={t.link} plain />}
        <WaitingBadge waiting={t.waiting_for_spec} />
        {needsYou && <span className="dot needs-you" role="img" aria-label="Needs you" />}
      </span>
      <RowLatestEvent row={t} now={now} className="row-detail" />
    </a>
  );
}
