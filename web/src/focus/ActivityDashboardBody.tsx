// The Activity Dashboard's body: what it shows, from the data it is given.
// Two blocks count the Tickets in Ready and In Progress, and the In
// Progress Tickets are listed below them, scoped to loc's Project, with
// Archived Projects left out. With ciRuns (CI Tracking on in scope), a
// third block, CI Running, counts the CI Runs still going that count
// towards a Ticket's CI result, and opens the CI view. Those runs are
// listed below the In Progress list, in the service's order (newest
// started first), or "No CI Runs running": each entry shows the run's
// workflow, Ticket, Project in All Projects, branch, short commit and how
// long it has been going, opens its Ticket in the Inbox and carries the
// run's GitHub link (code-central-278).
//
// A block pulses when its count changes. An entry pulses when its
// Ticket's count in changes goes up, and once when it joins the list
// after the body first shows. An entry shows its Ticket's CI badge when
// it has a CI result.
import { useEffect, useRef, useState } from "react";
import { numberOf } from "../api/api";
import type { ApiError, CIOverview, CIProvider, CIRunRow, State } from "../api/types";
import { Count } from "../controls/Count";
import { Empty } from "../controls/Empty";
import { ErrorBanner } from "../controls/ErrorBanner";
import { projectName } from "../controls/ProjectName";
import { RowLatestEvent } from "../LatestEvent";
import type { InboxData, ProjectTicket } from "../live";
import { STAGE_TITLE, stageOf } from "../requirement/progress";
import { CILink } from "../ci/CILink";
import { RunLink, runDuration } from "../ci/CIView";
import { CIBadge } from "../ticket/CIBadge";
import { runName, runStatus, shortCommit } from "../ticket/CISection";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { usePulse } from "./pulse";

// CIRunsRead is a read of the running CI Runs: neither data nor error
// before the first read.
export interface CIRunsRead {
  data?: CIOverview;
  error?: ApiError;
}

export interface ActivityDashboardBodyProps {
  now: Date;
  data: InboxData;
  // changes counts the ticket.changed notices for each Ticket since the
  // dashboard showed.
  changes: Record<string, number>;
  // ciRuns is the read of the running CI Runs in scope, none without CI
  // Tracking on in scope.
  ciRuns?: CIRunsRead;
  // loc's Project is the one in scope; none means All Projects.
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function ActivityDashboardBody({ now, data, changes, ciRuns, loc, navigate }: ActivityDashboardBodyProps) {
  // shown is whether the body has shown yet: an entry that mounts after
  // that has joined the list, and pulses once.
  const [shown, setShown] = useState(false);
  useEffect(() => setShown(true), []);
  const archived = new Set(data.projects.filter((p) => p.archived).map((p) => p.key));
  const ciRunningCount = ciRuns?.data?.runs.length;
  const tickets = data.tickets.filter((t) => (!loc.project || t.project === loc.project) && !archived.has(t.project));
  // Every Ticket in the State counts, Stage and blocked Tickets included,
  // so the In Progress count is the list's length.
  const inState = (s: State) => tickets.filter((t) => t.state === s);
  // A stable order, so the list doesn't jump about as events arrive.
  const inProgress = inState("In Progress").sort(
    (a, b) => a.project.localeCompare(b.project) || Number(numberOf(a.id)) - Number(numberOf(b.id)),
  );

  return (
    <section aria-label="Activity Dashboard" className="view-body">
      <div className="dashboard-blocks">
        <StateCount state="Ready" count={inState("Ready").length} />
        <StateCount state="In Progress" count={inProgress.length} />
        {/* Keyed on whether the count is known yet, so the first read doesn't pulse. */}
        {ciRuns && <CIRunningCount key={String(ciRunningCount === undefined)} count={ciRunningCount} loc={loc} navigate={navigate} />}
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
        <Empty>Nothing in progress.</Empty>
      )}
      {ciRuns && <CIRunningList now={now} data={data} runs={ciRuns} loc={loc} navigate={navigate} />}
    </section>
  );
}

// EntryProps is what the lists and their entries read from the body's props.
type EntryProps = Pick<ActivityDashboardBodyProps, "now" | "data" | "loc" | "navigate">;

// StateCount is one State's block: its count in big type, its State name under it.
// It pulses when the count differs from the one it last showed.
function StateCount({ state, count }: { state: State; count: number }) {
  const pulse = usePulse(`dashboard-block state-${state.replace(" ", "-").toLowerCase()}`, count);
  return (
    <div role="group" aria-label={state} {...pulse}>
      <Count n={count} big />
      <span className="dashboard-state">{state}</span>
    </div>
  );
}

// CIRunningCount is the CI Running block: the count of running CI Runs in
// big type, a dash until the first read, and its name under it. It opens
// the CI view in the body's scope, with Back to where it was, and pulses
// when the count differs from the one it last showed.
function CIRunningCount({ count, loc, navigate }: { count?: number } & Pick<EntryProps, "loc" | "navigate">) {
  const pulse = usePulse("dashboard-block ci-running", count);
  return (
    <div role="group" aria-label="CI Running" {...pulse}>
      <CILink loc={loc} navigate={navigate} className="dashboard-block-link">
        <Count n={count} big />
        <span className="dashboard-state">CI Running</span>
      </CILink>
    </div>
  );
}

// CIRunningList lists the running CI Runs in the service's order, or says
// there are none, or shows a failed read's error in their place. Nothing
// shows before the first read. The entries of the first read don't
// pulse; one that joins later pulses once.
interface CIRunningListProps extends EntryProps {
  runs: CIRunsRead;
}

function CIRunningList({ now, data, runs, loc, navigate }: CIRunningListProps) {
  const first = useRef<Set<string>>(undefined);
  if (runs.data && !first.current) first.current = new Set(runs.data.runs.map(runKey));
  if (runs.error) {
    return <ErrorBanner>{runs.error.message}</ErrorBanner>;
  }
  const overview = runs.data;
  if (!overview) return null;
  return overview.runs.length ? (
    <ul aria-label="CI Running" className="dashboard-list">
      {overview.runs.map((r) => (
        <CIRunEntry key={runKey(r)} now={now} data={data} run={r} provider={overview.projects.find((p) => p.project === r.project)?.provider} joined={!first.current?.has(runKey(r))} loc={loc} navigate={navigate} />
      ))}
    </ul>
  ) : (
    <Empty>No CI Runs running.</Empty>
  );
}

// runKey names a run across Projects; a re-run's next attempt is the same entry.
const runKey = (r: CIRunRow) => `${r.project}/${r.id}`;

// CIRunEntry is one running CI Run. Its Ticket link opens the run's Ticket
// as an In Progress entry does, and its link to the CI system sits beside it, since
// a link can't hold another. It pulses when the run's status or attempt
// changes, and on mounting if it joined the list.
interface CIRunEntryProps extends EntryProps {
  run: CIRunRow;
  provider?: CIProvider;
  joined: boolean;
}

function CIRunEntry({ now, data, run, provider, joined, loc, navigate }: CIRunEntryProps) {
  const pulse = usePulse("dashboard-ci-entry", `${run.status} ${run.attempt}`, joined);
  // The service lists only runs tracked against a Ticket.
  const inbox = inboxFor(data, run.project, run.ticket!, loc);
  return (
    <li aria-label={runName(run)} {...pulse}>
      <a href={hrefFor(inbox)} onClick={clickTo(navigate, inbox)}>
        <span className="dashboard-entry-top">
          <span className="tid">{run.ticket}</span>
          <span className="dashboard-entry-title">{runName(run)}</span>
          {!loc.project && <span className="muted">{nameOf(data, run.project)}</span>}
        </span>
        <span className="dashboard-entry-detail">
          {run.branch} · <code>{shortCommit(run.commit)}</code> · {runStatus(run)}
          {run.started && ` ${runDuration(run, now)}`}
        </span>
      </a>
      <RunLink run={run} provider={provider} className="dashboard-ci-github" />
    </li>
  );
}

// inboxFor is where a Ticket opens in the Inbox from the dashboard, as a
// Focus Item's "Open in Inbox" does: a Stage Ticket in its Requirement,
// with it selected, in the body's Project scope.
// A Ticket the Inbox data doesn't hold yet opens on its own.
function inboxFor(data: InboxData, project: string, id: string, loc: UiLocation): UiLocation {
  const t = data.tickets.find((t) => t.project === project && t.id === id);
  return { view: "inbox", project: loc.project, requirement: t && stageOf(t) && t.spec, ticket: id };
}

// nameOf is a Project's display name, or its key until the Inbox data holds it.
const nameOf = (data: InboxData, key: string) => projectName(data.projects, key);

// InProgressEntry is one In Progress Ticket. It opens in the Inbox by
// inboxFor. It pulses each time changes goes up, and on mounting
// if it joined the list.
interface InProgressEntryProps extends EntryProps {
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
          {!loc.project && <span className="muted">{nameOf(data, t.project)}</span>}
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
