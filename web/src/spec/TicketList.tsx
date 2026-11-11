// A Spec's Tickets as a list in its view: each one's ID and title to
// select it, its State, loud when it needs the human, its CI badge, and
// its live Claim's latest Session Event.
import type { NeedsMeReason, TicketRow } from "../api/types";
import { RowLatestEvent } from "../LatestEvent";
import { CIBadge } from "../ticket/CIBadge";
import { PullRequestBadge } from "../ticket/PullRequestBadge";
import { WaitingBadge } from "../ticket/WaitingBadge";

export interface TicketListProps {
  // label names the list.
  label: string;
  tickets: TicketRow[];
  reasons: Map<string, NeedsMeReason>;
  now: Date;
  // ticket is the selected Ticket.
  ticket?: string;
  select(ticket: string): void;
}

export function TicketList({ label, tickets, reasons, now, ticket, select }: TicketListProps) {
  return (
    <ul className="impl-list" aria-label={label}>
      {tickets.map((t) => {
        const reason = reasons.get(t.id);
        return (
          <li key={t.id}>
            <button className="link" aria-current={t.id === ticket ? "true" : undefined} onClick={() => select(t.id)}>
              <span className="tid">{t.id}</span> {t.title}
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
