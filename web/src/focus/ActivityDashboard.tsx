// The Activity Dashboard: what Focus Mode shows instead of All clear when
// the human has chosen it. Two blocks count the Tickets in Ready and In
// Progress, and the In Progress Tickets are listed below them. They come
// from the Inbox data Focus Mode already has, scoped as Focus Mode is,
// with Archived Projects left out. When a Project in scope that isn't
// Archived has CI Tracking on, a third block, CI Running, counts the CI
// Runs still going that count towards a Ticket's CI result: only then does
// the dashboard also read the running CI Runs, again on every live-feed
// change (code-central-277). The block opens the CI view. Those runs are
// listed below the In Progress list, newest started first, or "No CI
// Runs running": each entry shows the run's workflow, Ticket, Project in
// All Projects, branch, short commit and how long it has been going,
// opens its Ticket in the Inbox and carries the run's GitHub link
// (code-central-278).
//
// A block pulses when its count changes. An entry pulses when a
// ticket.changed notice names its Ticket, which includes each new Session
// Event, and once when it joins the list after the dashboard first shows.
// An entry shows its Ticket's CI badge when it has a CI result.
import { useEffect, useRef, useState } from "react";
import { numberOf, type Api } from "../api/api";
import type { ApiError, CIOverview, CIRunRow, State } from "../api/types";
import { RowLatestEvent } from "../LatestEvent";
import { useFetch, type InboxData, type ProjectTicket } from "../live";
import { STAGE_TITLE, stageOf } from "../requirement/progress";
import { CILink } from "../ci/CILink";
import { RunGitHubLink, runDuration } from "../ci/CIView";
import { CIBadge } from "../ticket/CIBadge";
import { runName, runStatus, shortCommit } from "../ticket/CISection";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { usePulse } from "./pulse";

export interface ActivityDashboardProps {
  api: Api;
  now: Date;
  // version is the live feed's, so the running CI Runs follow every change.
  version: number;
  data: InboxData;
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityDashboard({ api, now, version, data, loc, navigate }: ActivityDashboardProps) {
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
  const ciTracked = data.projects.some((p) => !p.archived && p.ci.tracking && (!loc.project || p.key === loc.project));
  const ciRuns = useFetch(ciTracked ? () => api.ciOverview({ project: loc.project, running: true }) : null, [api, loc.project, ciTracked, version]);
  const ciRunningCount = ciRuns.data?.runs.length;
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
        {/* Keyed on whether the count is known yet, so the first read doesn't pulse. */}
        {ciTracked && <CIRunningCount key={String(ciRunningCount === undefined)} count={ciRunningCount} loc={loc} navigate={navigate} />}
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
      {ciTracked && <CIRunningList now={now} data={data} runs={ciRuns} loc={loc} navigate={navigate} />}
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

// CIRunningCount is the CI Running block: the count of running CI Runs in
// big type, a dash until the first read, and its name under it. It opens
// the CI view in Focus Mode's scope, with Back to Focus Mode, and pulses
// when the count differs from the one it last showed.
function CIRunningCount({ count, loc, navigate }: { count?: number } & Pick<ActivityDashboardProps, "loc" | "navigate">) {
  const pulse = usePulse("dashboard-block ci-running", count);
  return (
    <div role="group" aria-label="CI Running" {...pulse}>
      <CILink loc={loc} navigate={navigate} className="dashboard-block-link">
        <span className="dashboard-count" data-testid="count">
          {count ?? "–"}
        </span>
        <span className="dashboard-state">CI Running</span>
      </CILink>
    </div>
  );
}

// CIRunningList lists the running CI Runs in the service's order, or says
// there are none, or shows a failed read's error in their place. Nothing
// shows before the first read. The entries of the first read don't
// pulse; one that joins later pulses once.
interface CIRunningListProps extends Omit<ActivityDashboardProps, "api" | "version"> {
  runs: { data?: CIOverview; error?: ApiError };
}

function CIRunningList({ now, data, runs, loc, navigate }: CIRunningListProps) {
  const first = useRef<Set<string>>(undefined);
  if (runs.data && !first.current) first.current = new Set(runs.data.runs.map(runKey));
  if (runs.error) {
    return (
      <p role="alert" className="warning">
        {runs.error.message}
      </p>
    );
  }
  if (!runs.data) return null;
  return runs.data.runs.length ? (
    <ul aria-label="CI Running" className="dashboard-list">
      {runs.data.runs.map((r) => (
        <CIRunEntry key={runKey(r)} now={now} data={data} run={r} joined={!first.current?.has(runKey(r))} loc={loc} navigate={navigate} />
      ))}
    </ul>
  ) : (
    <p className="dashboard-empty">No CI Runs running</p>
  );
}

// runKey names a run across Projects; a re-run's next attempt is the same entry.
const runKey = (r: CIRunRow) => `${r.project}/${r.id}`;

// CIRunEntry is one running CI Run. Its Ticket link opens the run's Ticket
// as an In Progress entry does, and its GitHub link sits beside it, since
// a link can't hold another. It pulses when the run's status or attempt
// changes, and on mounting if it joined the list.
interface CIRunEntryProps extends Omit<ActivityDashboardProps, "api" | "version"> {
  run: CIRunRow;
  joined: boolean;
}

function CIRunEntry({ now, data, run, joined, loc, navigate }: CIRunEntryProps) {
  const pulse = usePulse("dashboard-ci-entry", `${run.status} ${run.attempt}`, joined);
  // The service lists only runs tracked against a Ticket.
  const inbox = inboxFor(data, run.project, run.ticket!, loc);
  return (
    <li aria-label={runName(run)} {...pulse}>
      <a href={hrefFor(inbox)} onClick={clickTo(navigate, inbox)}>
        <span className="dashboard-entry-top">
          <span className="tid">{run.ticket}</span>
          <span className="dashboard-entry-title">{runName(run)}</span>
          {!loc.project && <span className="muted">{projectName(data, run.project)}</span>}
        </span>
        <span className="dashboard-entry-detail">
          {run.branch} · <code>{shortCommit(run.commit)}</code> · {runStatus(run)}
          {run.started && ` ${runDuration(run, now)}`}
        </span>
      </a>
      <RunGitHubLink run={run} className="dashboard-ci-github" />
    </li>
  );
}

// inboxFor is where a Ticket opens in the Inbox from the dashboard, as a
// Focus Item's "Open in Inbox" does: a Stage Ticket in its Requirement,
// with it selected, in Focus Mode's Project scope.
// A Ticket the Inbox data doesn't hold yet opens on its own.
function inboxFor(data: InboxData, project: string, id: string, loc: UiLocation): UiLocation {
  const t = data.tickets.find((t) => t.project === project && t.id === id);
  return { view: "inbox", project: loc.project, requirement: t && stageOf(t) && t.spec, ticket: id };
}

// projectName is a Project's display name, or its key until the Inbox data holds it.
const projectName = (data: InboxData, key: string) => data.projects.find((p) => p.key === key)?.display_name ?? key;

// InProgressEntry is one In Progress Ticket. It opens in the Inbox by
// inboxFor. It pulses each time changes goes up, and on mounting
// if it joined the list.
interface InProgressEntryProps extends Omit<ActivityDashboardProps, "api" | "version"> {
  ticket: ProjectTicket;
  changes: number;
  joined: boolean;
}

function InProgressEntry({ now, data, ticket: t, changes, joined, loc, navigate }: InProgressEntryProps) {
  const pulse = usePulse("dashboard-entry", changes, joined);
  const stage = stageOf(t);
  const requirement = stage && data.requirements.find((s) => s.id === t.spec);
  const inbox = inboxFor(data, t.project, t.id, loc);
  return (
    <li {...pulse}>
      <a href={hrefFor(inbox)} onClick={clickTo(navigate, inbox)}>
        <span className="dashboard-entry-top">
          <span className="tid">{t.id}</span>
          <span className="dashboard-entry-title">{t.title}</span>
          <CIBadge result={t.ci_result} />
          {!loc.project && <span className="muted">{projectName(data, t.project)}</span>}
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
