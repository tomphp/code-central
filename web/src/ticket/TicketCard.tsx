// The Ticket card: a Ticket on the Board. Its top line is the Short ID, an
// optional Stage badge, the CI badge (none on a Done Ticket), Partly
// landed, Blocked and the needs-you dot; then its title, then
// its Changed Repositories' chips while the Project has several, live
// while it is In Progress, then
// its Spec's kind and title when given (a lane's cards leave it out). The
// whole card is one link. In the Needs Me column the top line also has the
// State chip and the Reason badge. It pulses when it joins its column, and when its
// Ticket's last update changes.
import type { MouseEvent } from "react";
import { Blocked } from "../controls/Icon";
import type { TicketRow } from "../api/types";
import { KindBadge } from "../controls/KindBadge";
import { NeedsYouDot, useNeedsMe } from "../needsme/context";
import { StageBadge } from "../controls/StageBadge";
import { usePulse } from "../focus/pulse";
import { ReasonBadge } from "../controls/ReasonBadge";
import { RepositoryChips } from "../controls/RepositoryChips";
import { StateChip } from "../controls/StateChip";
import { ShortId } from "../inbox/ShortId";
import { ListCIBadge } from "./CIBadge";
import { PartlyLandedBadge } from "./PartlyLandedBadge";

// TicketCardProps are a Ticket card's Ticket, its link, and the parts the
// Board shows.
export interface TicketCardProps {
  t: Pick<TicketRow, "id" | "title" | "blocked" | "updated" | "state" | "ci_result" | "repositories" | "partly_landed">;
  link: { href: string; onClick(e: MouseEvent): void };
  // badge is the Ticket's Stage or other kind, as a Stage badge.
  badge?: string;
  spec?: { kind: string; title: string };
  // joined pulses the card as it mounts: its Ticket changed, or is new,
  // since the Board last rendered.
  joined?: boolean;
  // inNeedsMe is a card in the Board's Needs Me column: its top line adds
  // the Ticket's State chip and its Reason badge.
  inNeedsMe?: boolean;
}

// TicketCard is the Ticket card.
export function TicketCard({ t, link, badge, spec, joined, inNeedsMe }: TicketCardProps) {
  const reason = useNeedsMe().reasonOf(t.id);
  const pulse = usePulse(reason ? `board-card needs reason-${reason}` : "board-card", t.updated, joined, "board");
  return (
    <a {...pulse} {...link}>
      <span className="board-card-top">
        <ShortId id={t.id} />
        {inNeedsMe && <StateChip state={t.state} />}
        {inNeedsMe && reason && <ReasonBadge reason={reason} />}
        {badge && <StageBadge label={badge} />}
        <span className="board-spacer" />
        <ListCIBadge t={t} />
        <PartlyLandedBadge partly={t.partly_landed} />
        {t.blocked && (
          <span className="board-blocked" title="Has open Blockers">
            <Blocked /> Blocked
          </span>
        )}
        <NeedsYouDot id={t.id} />
      </span>
      <span className="board-card-title">{t.title}</span>
      <RepositoryChips names={t.repositories} />
      {spec && (
        <span className="board-card-spec">
          <KindBadge kind={spec.kind} /> {spec.title}
        </span>
      )}
    </a>
  );
}
