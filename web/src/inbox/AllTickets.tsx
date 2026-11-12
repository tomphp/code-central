// The All Tickets view's By Ticket list: every Ticket in scope, flat, newest
// first by creation, Stage Tickets inline. Each row is the State list's
// two-line row with a State chip and a kind label, and names its Spec on the
// second line.
import type { NeedsMeReason } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { stageOf } from "../requirement/progress";
import { LinkBadge } from "../ticket/LinkBadge";
import { WaitingBadge } from "../ticket/WaitingBadge";
import { specKindOf } from "./groupBySpec";
import { ShortId } from "./ShortId";
import type { InboxRowLinkFactory } from "./StateRow";

const slug = (s: string) => s.toLowerCase().replace(/\s+/g, "-");
const numberOf = (id: string) => Number(id.slice(id.lastIndexOf("-") + 1));

// newestFirst orders Tickets by creation, then Project, then number, all
// descending.
export function newestFirst(tickets: ProjectTicket[]): ProjectTicket[] {
  return [...tickets].sort(
    (a, b) =>
      b.created.localeCompare(a.created) ||
      b.project.localeCompare(a.project) ||
      numberOf(b.id) - numberOf(a.id),
  );
}

// kindLabel is the Stage name, "Prototype", or a Map node type other than
// `ticket`; none for implementation and Fix Tickets.
function kindLabel(t: ProjectTicket): string | undefined {
  if (stageOf(t)) return stageOf(t);
  if (t.labels.includes("prototype")) return "Prototype";
  return t.node_type && t.node_type !== "ticket" ? t.node_type : undefined;
}

export function AllTicketsByTicket({
  tickets,
  specs,
  reasons,
  link,
}: {
  tickets: ProjectTicket[];
  specs: Map<string, ProjectSpec>;
  reasons: Map<string, NeedsMeReason>;
  link: InboxRowLinkFactory;
}) {
  if (tickets.length === 0) return <p className="empty">No unfinished Tickets.</p>;
  return (
    <>
      {newestFirst(tickets).map((t) => {
        const spec = specs.get(t.spec);
        const label = kindLabel(t);
        return (
          <a key={t.id} className="row" {...link(t.id)}>
            <span className="row-top">
              <ShortId id={t.id} />
              {label && <span className="bs-stage">{label}</span>}
              <span className="row-title">{t.title}</span>
              {t.link && <LinkBadge link={t.link} plain />}
              <WaitingBadge waiting={t.waiting_for_spec} />
              <span className={`bs-chip st-${slug(t.state)}`}>{t.state}</span>
              {reasons.has(t.id) && <span className="dot needs-you" role="img" aria-label="Needs you" />}
            </span>
            <span className="row-detail">
              <span className={`bs-kind bs-kind-${specKindOf(spec).toLowerCase()}`}>{specKindOf(spec)}</span> <ShortId id={t.spec} /> {spec?.title ?? ""}
            </span>
          </a>
        );
      })}
    </>
  );
}
