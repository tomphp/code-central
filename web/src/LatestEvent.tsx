// The latest Session Event of a Claim, shown short and faintly: "<text> ·
// <ago>". Rows show it only for In Progress Tickets.
import type { LatestEvent, TicketRow } from "./api/types";
import { ago } from "./format";

export function LatestEventLine({ event, now, className }: { event: LatestEvent; now: Date; className?: string }) {
  return <span className={["latest-event", className].filter(Boolean).join(" ")}>{`${event.text} · ${ago(now, event.time)}`}</span>;
}

// RowLatestEvent is an In Progress row's latest event; nothing on any other row.
export function RowLatestEvent({ row, now, className }: { row: Pick<TicketRow, "state" | "latest_event">; now: Date; className?: string }) {
  return row.state === "In Progress" && row.latest_event ? <LatestEventLine event={row.latest_event} now={now} className={className} /> : null;
}
