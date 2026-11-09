// The Activity Dashboard: what Focus Mode shows instead of All clear when
// the human has chosen it. Two blocks count the Tickets in Ready and In
// Progress, and the In Progress Tickets are listed below them. It reads
// only the Inbox data Focus Mode already has, scoped as Focus Mode is,
// with Archived Projects left out.
import { numberOf } from "../api/api";
import type { State } from "../api/types";
import { RowLatestEvent } from "../LatestEvent";
import type { InboxData, ProjectTicket } from "../live";
import { STAGE_TITLE, stageOf } from "../requirement/progress";
import { linkTo, type UiLocation } from "../url";

export interface ActivityDashboardProps {
  now: Date;
  data: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityDashboard({ now, data, loc, navigate }: ActivityDashboardProps) {
  const archived = new Set(data.projects.filter((p) => p.archived).map((p) => p.key));
  const tickets = data.tickets.filter((t) => (!loc.project || t.project === loc.project) && !archived.has(t.project));
  // Every Ticket in the State counts, Stage and blocked Tickets included,
  // so the In Progress count is the list's length.
  const inState = (s: State) => tickets.filter((t) => t.state === s);
  // A stable order, so the list doesn't jump about as events arrive.
  const inProgress = inState("In Progress").sort(
    (a, b) => a.project.localeCompare(b.project) || Number(numberOf(a.id)) - Number(numberOf(b.id)),
  );

  return (
    <section aria-label="Activity Dashboard" className="dashboard">
      <div className="dashboard-blocks">
        <StateCount state="Ready" count={inState("Ready").length} />
        <StateCount state="In Progress" count={inProgress.length} />
      </div>
      {inProgress.length ? (
        <ul aria-label="In Progress" className="dashboard-list">
          {inProgress.map((t) => (
            <InProgressEntry key={t.id} now={now} data={data} ticket={t} loc={loc} navigate={navigate} />
          ))}
        </ul>
      ) : (
        <p className="dashboard-empty">Nothing in progress</p>
      )}
    </section>
  );
}

// StateCount is one State's block: its count in big type, its State name under it.
function StateCount({ state, count }: { state: State; count: number }) {
  return (
    <div role="group" aria-label={state} className={`dashboard-block state-${state.replace(" ", "-").toLowerCase()}`}>
      <span className="dashboard-count" data-testid="count">
        {count}
      </span>
      <span className="dashboard-state">{state}</span>
    </div>
  );
}

// InProgressEntry is one In Progress Ticket. It opens in the Inbox as a
// Focus Item's "Open in Inbox" does: a Stage Ticket in its Requirement,
// with it selected.
function InProgressEntry({ now, data, ticket: t, loc, navigate }: ActivityDashboardProps & { ticket: ProjectTicket }) {
  const stage = stageOf(t);
  const requirement = stage && data.requirements.find((s) => s.id === t.spec);
  const inbox = linkTo({ view: "inbox", project: loc.project, requirement: stage && t.spec, ticket: t.id }, navigate);
  return (
    <li className="dashboard-entry">
      <a {...inbox}>
        <span className="dashboard-entry-top">
          <span className="tid">{t.id}</span>
          <span className="dashboard-entry-title">{t.title}</span>
          {!loc.project && <span className="muted">{data.projects.find((p) => p.key === t.project)?.display_name ?? t.project}</span>}
        </span>
        {stage && (
          <span className="dashboard-entry-detail">
            Requirement: {requirement?.title ?? t.spec} · {STAGE_TITLE[stage]}
          </span>
        )}
        <RowLatestEvent row={t} now={now} className="dashboard-entry-detail" />
      </a>
    </li>
  );
}
