// The Ticket card: a Ticket on the Board. Its top line is the Short ID, an
// optional Stage badge, the CI badge (none on a Done Ticket), Blocked and the needs-you dot; then its title, then
// its Spec's kind and title when given (a lane's cards leave it out). The
// whole card is one link. In the Needs Me column the top line also has the
// State chip and the reason's label. It pulses when it joins its column, and when its
// Ticket's last update changes.
import type { MouseEvent } from "react";
import type { NeedsMeReason, TicketRow } from "../api/types";
import { KindBadge } from "../controls/KindBadge";
import { StageBadge } from "../controls/StageBadge";
import { usePulse } from "../focus/pulse";
import { StateChip } from "../controls/StateChip";
import { REASON_LABEL } from "../format";
import { ShortId } from "../inbox/ShortId";
import { ListCIBadge } from "./CIBadge";

// TicketCardProps are a Ticket card's Ticket, its link, and the parts the
// Board shows.
export interface TicketCardProps {
  t: Pick<TicketRow, "id" | "title" | "blocked" | "updated" | "state" | "ci_result">;
  link: { href: string; onClick(e: MouseEvent): void };
  // badge is the Ticket's Stage or other kind, as a Stage badge.
  badge?: string;
  // reason is why the Ticket needs the human, if it does.
  reason?: NeedsMeReason;
  spec?: { kind: string; title: string };
  // joined pulses the card as it mounts: its Ticket changed, or is new,
  // since the Board last rendered.
  joined?: boolean;
  // inNeedsMe is a card in the Board's Needs Me column: its top line adds
  // the Ticket's State chip and its reason's label.
  inNeedsMe?: boolean;
}

// TicketCard is the Ticket card.
export function TicketCard({ t, link, badge, reason, spec, joined, inNeedsMe }: TicketCardProps) {
  const pulse = usePulse(reason ? `board-card needs reason-${reason}` : "board-card", t.updated, joined, "board");
  return (
    <a {...pulse} {...link}>
      <span className="board-card-top">
        <ShortId id={t.id} />
        {inNeedsMe && <StateChip state={t.state} />}
        {inNeedsMe && reason && <span className="board-reason">{REASON_LABEL[reason]}</span>}
        {badge && <StageBadge label={badge} />}
        <span className="board-spacer" />
        <ListCIBadge t={t} />
        {t.blocked && (
          <span className="board-blocked" title="Has open Blockers">
            ⛔ Blocked
          </span>
        )}
        {reason && <span className="dot" role="img" aria-label="Needs you" />}
      </span>
      <span className="board-card-title">{t.title}</span>
      {spec && (
        <span className="board-card-spec">
          <KindBadge kind={spec.kind} /> {spec.title}
        </span>
      )}
    </a>
  );
}
