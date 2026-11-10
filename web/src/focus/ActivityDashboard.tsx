// The Activity Dashboard: what Focus Mode shows instead of All clear when
// the human has chosen it. Two blocks count the Tickets in Ready and In
// Progress, and the In Progress Tickets are listed below them. It reads
// only the Inbox data Focus Mode already has, scoped as Focus Mode is,
// with Archived Projects left out.
//
// A block pulses when its count changes. An entry pulses when a
// ticket.changed notice names its Ticket, which includes each new Session
// Event, and once when it joins the list after the dashboard first shows.
// An entry shows its Ticket's CI badge when it has a CI result.
import { useEffect, useState } from "react";
import { numberOf, type Api } from "../api/api";
import type { State } from "../api/types";
import { RowLatestEvent } from "../LatestEvent";
import type { InboxData, ProjectTicket } from "../live";
import { STAGE_TITLE, stageOf } from "../requirement/progress";
import { CIBadge } from "../ticket/CIBadge";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { usePulse } from "./pulse";

export interface ActivityDashboardProps {
  api: Api;
  now: Date;
  data: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityDashboard({ api, now, data, loc, navigate }: ActivityDashboardProps) {
  // changes counts the ticket.changed notices for each Ticket since the
  // dashboard showed: the version number alone doesn't say which Ticket
  // changed, so the dashboard follows the live feed itself.
  const [changes, setChanges] = useState<Record<string, number>>({});
  useEffect(
    () =>
      api.subscribe({
        onEvent: (e) => e.type === "ticket.changed" && setChanges((c) => ({ ...c, [e.id]: (c[e.id] ?? 0) + 1 })),
      }),
    [api],
  );
  // shown is whether the dashboard has shown yet: an entry that mounts
  // after that has joined the list, and pulses once.
  const [shown, setShown] = useState(false);
  useEffect(() => setShown(true), []);
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
            <InProgressEntry
              key={t.id}
              now={now}
              data={data}
              ticket={t}
              changes={changes[t.id] ?? 0}
              joined={shown}
              loc={loc}
              navigate={navigate}
            />
          ))}
        </ul>
      ) : (
        <p className="dashboard-empty">Nothing in progress</p>
      )}
    </section>
  );
}

// StateCount is one State's block: its count in big type, its State name under it.
// It pulses when the count differs from the one it last showed.
function StateCount({ state, count }: { state: State; count: number }) {
  const pulse = usePulse(`dashboard-block state-${state.replace(" ", "-").toLowerCase()}`, count);
  return (
    <div role="group" aria-label={state} {...pulse}>
      <span className="dashboard-count" data-testid="count">
        {count}
      </span>
      <span className="dashboard-state">{state}</span>
    </div>
  );
}

// InProgressEntry is one In Progress Ticket. It opens in the Inbox as a
// Focus Item's "Open in Inbox" does: a Stage Ticket in its Requirement,
// with it selected. It pulses each time changes goes up, and on mounting
// if it joined the list.
interface InProgressEntryProps extends Omit<ActivityDashboardProps, "api"> {
  ticket: ProjectTicket;
  changes: number;
  joined: boolean;
}

function InProgressEntry({ now, data, ticket: t, changes, joined, loc, navigate }: InProgressEntryProps) {
  const pulse = usePulse("dashboard-entry", changes, joined);
  const stage = stageOf(t);
  const requirement = stage && data.requirements.find((s) => s.id === t.spec);
  const inbox = { view: "inbox" as const, project: loc.project, requirement: stage && t.spec, ticket: t.id };
  return (
    <li {...pulse}>
      <a href={hrefFor(inbox)} onClick={clickTo(navigate, inbox)}>
        <span className="dashboard-entry-top">
          <span className="tid">{t.id}</span>
          <span className="dashboard-entry-title">{t.title}</span>
          <CIBadge result={t.ci_result} />
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
