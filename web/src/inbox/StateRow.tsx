// A State list's two-line Ticket row, shared by the By Ticket and By Spec
// groupings: the latest Session Event under the title, and a dot when the
// Ticket needs the human.
import type { MouseEvent } from "react";
import { RowLatestEvent } from "../LatestEvent";
import type { ProjectTicket } from "../live";
import { stageOf } from "../requirement/progress";
import { LinkBadge } from "../ticket/LinkBadge";

export type InboxRowLinkFactory = (id: string) => {
  href: string;
  onClick(e: MouseEvent): void;
  "data-id": string;
  "aria-current"?: "true";
};

// stage shows the Ticket's Stage as a badge, for a Stage Ticket sitting
// among its Requirement's Tickets.
export function StateRow({ t, needsYou, now, link, stage }: { t: ProjectTicket; needsYou: boolean; now: Date; link: InboxRowLinkFactory; stage?: boolean }) {
  const name = stage ? stageOf(t) : undefined;
  return (
    <a className="row" {...link(t.id)}>
      <span className="row-top">
        <span className="tid">{t.id}</span>
        {name && <span className="bs-stage">{name}</span>}
        <span className="row-title">{t.title}</span>
        {t.link && <LinkBadge link={t.link} plain />}
        {needsYou && <span className="dot needs-you" role="img" aria-label="Needs you" />}
      </span>
      <RowLatestEvent row={t} now={now} className="row-detail" />
    </a>
  );
}
