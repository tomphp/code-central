// The Ticket row: the two-line row a list shows a Ticket in. Its first line
// is the Short ID, an optional Stage badge, the title, an optional State
// chip, the Link and Waiting badges, any badges the list adds, and a dot
// when the Ticket needs the human; its second line is the latest Session
// Event, or the detail the list gives. The whole row is one link.
import type { MouseEvent, ReactNode } from "react";
import type { TicketRow } from "../api/types";
import { StageBadge } from "../controls/StageBadge";
import { StateChip } from "../controls/StateChip";
import { ShortId } from "../inbox/ShortId";
import { RowLatestEvent } from "../LatestEvent";
import { LinkBadge } from "./LinkBadge";
import { WaitingBadge } from "./WaitingBadge";

// RowLink is what a row's link spreads on itself.
export interface RowLink {
  href: string;
  onClick(e: MouseEvent): void;
  "data-id": string;
  "aria-current"?: "true";
}

// InboxRowLinkFactory is a list's link to each of its Tickets, by ID.
export type InboxRowLinkFactory = (id: string) => RowLink;

// TicketLineTicket is what a row shows of its Ticket; a Needs Me row has
// it too.
export type TicketLineTicket = Pick<TicketRow, "id" | "title" | "state" | "link" | "waiting_for_spec" | "latest_event">;

export interface TicketLineProps {
  t: TicketLineTicket;
  link: RowLink;
  // badge is the Ticket's Stage or other kind, as a Stage badge.
  badge?: string;
  // chip shows the Ticket's State, in a list of several States.
  chip?: boolean;
  needsYou?: boolean;
  // quiet dims the row, as Drafts' rows are.
  quiet?: boolean;
  // now shows the latest Session Event on the second line; detail replaces
  // it. With neither, the row is one line.
  now?: Date;
  detail?: ReactNode;
  // children are the list's own badges, after the Waiting badge.
  children?: ReactNode;
}

export function TicketLine({ t, link, badge, chip, needsYou, quiet, now, detail, children }: TicketLineProps) {
  return (
    <a className={quiet ? "row quiet" : "row"} {...link}>
      <span className="row-top">
        <ShortId id={t.id} />
        {badge && <StageBadge label={badge} />}
        <span className="row-title">{t.title}</span>
        {chip && <StateChip state={t.state} />}
        {t.link && <LinkBadge link={t.link} plain />}
        <WaitingBadge waiting={t.waiting_for_spec} />
        {children}
        {needsYou && <span className="dot needs-you" role="img" aria-label="Needs you" />}
      </span>
      {detail !== undefined ? <span className="row-detail">{detail}</span> : now && <RowLatestEvent row={t} now={now} className="row-detail" />}
    </a>
  );
}

// TicketName is a Ticket's Short ID and title inline, for a list whose
// item is a button rather than a row: a Spec's Ticket list and a Map's
// nodes.
export function TicketName({ t }: { t: Pick<TicketRow, "id" | "title"> }) {
  return (
    <>
      <ShortId id={t.id} /> {t.title}
    </>
  );
}
