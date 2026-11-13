// A State list's two-line Ticket row, shared by the By Ticket grouping and
// the Spec row: the latest Session Event under the title, and a dot when the
// Ticket needs the human.
import type { MouseEvent } from "react";
import { StateChip, stateSlug } from "../controls/StateChip";
import { StageBadge } from "../controls/StageBadge";
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

// stateSlug moved to controls/StateChip; re-exported until no importer is left.
export { stateSlug };

// stage shows the Ticket's Stage as a badge, for a Stage Ticket sitting
// among its Requirement's Tickets; chip shows its State, in a list of
// several States.
export function StateRow({ t, needsYou, now, link, stage, chip }: { t: ProjectTicket; needsYou: boolean; now: Date; link: InboxRowLinkFactory; stage?: boolean; chip?: boolean }) {
  const name = stage ? stageOf(t) : undefined;
  return (
    <a className="row" {...link(t.id)}>
      <span className="row-top">
        <ShortId id={t.id} />
        {name && <StageBadge label={name} />}
        <span className="row-title">{t.title}</span>
        {chip && <StateChip state={t.state} />}
        {t.link && <LinkBadge link={t.link} plain />}
        <WaitingBadge waiting={t.waiting_for_spec} />
        {needsYou && <span className="dot needs-you" role="img" aria-label="Needs you" />}
      </span>
      <RowLatestEvent row={t} now={now} className="row-detail" />
    </a>
  );
}
