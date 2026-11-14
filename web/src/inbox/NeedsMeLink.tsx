import { duration, rowDetail } from "../format";
import type { NeedsMeRow } from "../api/types";
import { CIBadge } from "../ticket/CIBadge";
import { PullRequestBadge } from "../ticket/PullRequestBadge";
import { PrototypeBadge } from "../ticket/PrototypeBadge";
import { TicketLine, type RowLink } from "../ticket/TicketLine";

export function NeedsMeLink({
  row,
  now,
  link,
}: {
  row: NeedsMeRow;
  now: Date;
  link: RowLink;
}) {
  // A Project's Base diverged has no Ticket to open. A CI item has none
  // of its own either: it opens on the first Ticket it lists, whose panel
  // shows the item and its options (code-central-432).
  if (!row.id) {
    const top = (
      <span className="row-top">
        <span className="tid">{row.project}</span>
        <span className="row-title">{row.title}</span>
        {row.ci_item && <CIBadge result={row.ci_result} />}
        <span className="muted small">{duration(now, row.since)}</span>
      </span>
    );
    if (row.ci_item) {
      return (
        <a className="row" {...link}>
          {top}
          <span className="row-detail">{rowDetail(now, row)}</span>
        </a>
      );
    }
    return (
      <div className="row">
        {top}
        <span className="row-detail">{rowDetail(now, row)}</span>
      </div>
    );
  }
  return (
    <TicketLine
      t={row}
      link={link}
      inNeedsMe
      detail={
        <>
          {row.requirement && (
            <span className="req-chip">↳ {row.requirement.title}</span>
          )}
          {row.spec && (
            <span className="req-chip">
              ↳ {row.spec.map ? `Map: ${row.spec.title}` : row.spec.title}
            </span>
          )}{" "}
          {rowDetail(now, row)}
        </>
      }
    >
      {row.prototype && <PrototypeBadge />}
      <CIBadge result={row.ci_result} />
      <PullRequestBadge pr={row.pull_request} inRowLink />
      <span className="muted small">{duration(now, row.since)}</span>
    </TicketLine>
  );
}
