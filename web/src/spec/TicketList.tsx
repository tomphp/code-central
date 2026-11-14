// A Spec's Tickets as a list in its view: each one's ID and title to
// select it, its State, loud when it needs the human, its CI badge, and
// its live Claim's latest Session Event.
import type { TicketRow } from "../api/types";
import { RowLatestEvent } from "../LatestEvent";
import { useNeedsMe } from "../needsme/context";
import { CIBadge } from "../ticket/CIBadge";
import { PullRequestBadge } from "../ticket/PullRequestBadge";
import { WaitingBadge } from "../ticket/WaitingBadge";
import { TicketName } from "../ticket/TicketLine";

export interface TicketListProps {
  // label names the list.
  label: string;
  tickets: TicketRow[];
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function TicketList({ label, tickets, now, ticket, select }: TicketListProps) {
  const { reasonOf } = useNeedsMe();
  return (
    <ul className="impl-list" aria-label={label}>
      {tickets.map((t) => {
        const reason = reasonOf(t.id);
        return (
          <li key={t.id}>
            <button className="link" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
              <TicketName t={t} />
            </button>{" "}
            <span className={["state", reason && `reason-${reason} loud-state`].filter(Boolean).join(" ")}>{t.state}</span>
            <WaitingBadge waiting={t.waiting_for_spec} />
            <CIBadge result={t.ci_result} />
            <PullRequestBadge pr={t.pull_request} />
            <RowLatestEvent row={t} now={now} />
          </li>
        );
      })}
    </ul>
  );
}
