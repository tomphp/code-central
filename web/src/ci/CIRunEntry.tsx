// CIRunEntry is one CI Run as a CI Run display shows it (Spec
// code-central-650), composed of the shared CI Run parts, in one of three
// layouts:
//   - row: a <tr> in the CI view's table, with its workflow, branch, short
//     commit, status, duration, the Ticket or CI Rule it is tracked
//     against, its Project in All Projects and its link to the CI system.
//   - item: an <li> in a Ticket's CI section, its name linking the run,
//     with its workflow, status, attempt, short commit and jobs.
//   - card: an <li> on the Activity Dashboard, opening its Ticket, with its
//     workflow, Project in All Projects, branch, short commit, status and
//     how long it has run, and its link to the CI system beside it.
// The row and item grey an ignored run and mark it ignored; the card's
// list holds only runs that count, so it has neither. The row and card
// pulse when the run changes, and once on mounting if it joined the list.
import type { CIProvider, CIRunRow, CIRunView } from "../api/types";
import { ShortId } from "../inbox/ShortId";
import { usePulse } from "../focus/pulse";
import { runDuration, shortCommit } from "../format";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { CONCLUSION_LABEL, IgnoredMark, RunLink, RunStatusBadge, runName, runStatus } from "./RunParts";

// runKey names a run across Projects; a re-run's next attempt is the same run.
export const runKey = (r: CIRunRow) => `${r.project}/${r.id}`;

// CIRunListedProps are the props of a run listed across Projects: the
// row's and the card's.
interface CIRunListedProps {
  run: CIRunRow;
  provider?: CIProvider;
  now: Date;
  // joined is a run that wasn't in the list's first read: it pulses once.
  joined?: boolean;
  // projectName is the run's Project's name, in All Projects only.
  projectName?: string;
  navigate(patch: Partial<UiLocation>): void;
}

export type CIRunRowProps = CIRunListedProps;

export interface CIRunItemProps {
  run: CIRunView;
}

export interface CIRunCardProps extends CIRunListedProps {
  // to is where its Ticket opens.
  to: UiLocation;
}

export type CIRunEntryProps = ({ layout: "row" } & CIRunRowProps) | ({ layout: "item" } & CIRunItemProps) | ({ layout: "card" } & CIRunCardProps);

export function CIRunEntry(props: CIRunEntryProps) {
  switch (props.layout) {
    case "row":
      return <RunRow {...props} />;
    case "item":
      return <RunItem {...props} />;
    case "card":
      return <RunCard {...props} />;
  }
}

// entryClass is the classes of an entry of className: ci-entry, and
// ignored for an ignored run, greys it.
const entryClass = (run: CIRunView, className: string) => `ci-entry ${className}${run.ignored ? " ignored" : ""}`;

// RunRow pulses when the run's status, conclusion or attempt changes.
function RunRow({ run, provider, now, joined = false, projectName, navigate }: CIRunRowProps) {
  const pulse = usePulse(entryClass(run, "ci-row"), `${run.status} ${run.conclusion ?? ""} ${run.attempt}`, joined);
  const tracked = run.ticket ?? run.rule;
  const to: UiLocation = run.ticket
    ? { view: "inbox", project: run.project, ticket: run.ticket }
    : { view: "inbox", project: run.project, requirement: run.rule };
  return (
    <tr {...pulse} aria-label={runName(run)}>
      <th scope="row">
        {runName(run)} <span className="muted small">{run.workflow}</span>
      </th>
      <td>{run.branch}</td>
      <td>
        <code>{shortCommit(run.commit)}</code>
      </td>
      <td>
        <RunStatusBadge status={runStatus(run)} />
        {run.attempt > 1 && <span className="muted small"> · attempt {run.attempt}</span>}
        {run.ignored && <IgnoredMark />}
      </td>
      <td className="num">{runDuration(run, now)}</td>
      <td>
        {tracked && (
          <>
            {run.rule && "CI Rule "}
            <a href={hrefFor(to)} onClick={clickTo(navigate, to)}>
              {tracked}
            </a>
          </>
        )}
      </td>
      {projectName !== undefined && <td>{projectName}</td>}
      <td>
        <RunLink run={run} provider={provider} />
      </td>
    </tr>
  );
}

const jobStatus = (job: CIRunView["jobs"][number]) => (job.conclusion ? CONCLUSION_LABEL[job.conclusion] : "running");

function RunItem({ run }: CIRunItemProps) {
  return (
    <li className={entryClass(run, "ci-run")} aria-label={runName(run)}>
      <div>
        <a href={run.url} target="_blank" rel="noreferrer">
          {runName(run)}
        </a>{" "}
        <span className="muted small">{run.workflow}</span> <RunStatusBadge status={runStatus(run)} />
        {run.attempt > 1 && <span className="muted small"> · attempt {run.attempt}</span>}
        {run.commit && <span className="muted small"> · {shortCommit(run.commit)}</span>}
        {run.ignored && <IgnoredMark />}
      </div>
      {run.jobs.length > 0 && (
        <ul className="ci-jobs small">
          {run.jobs.map((job, i) => (
            <li key={i}>
              <a href={job.url} target="_blank" rel="noreferrer">
                {job.name}
              </a>{" "}
              <span className="muted">{jobStatus(job)}</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

// RunCard's Ticket link opens the run's Ticket, and its link to the CI
// system sits beside it, since a link can't hold another. It pulses when
// the run's status or attempt changes.
function RunCard({ run, provider, now, joined = false, projectName, to, navigate }: CIRunCardProps) {
  const pulse = usePulse("dashboard-ci-entry", `${run.status} ${run.attempt}`, joined);
  return (
    <li aria-label={runName(run)} {...pulse}>
      <a href={hrefFor(to)} onClick={clickTo(navigate, to)}>
        <span className="dashboard-entry-top">
          <ShortId id={run.ticket!} />
          <span className="dashboard-entry-title">{runName(run)}</span>
          {projectName !== undefined && <span className="muted">{projectName}</span>}
        </span>
        <span className="dashboard-entry-detail">
          {run.branch} · <code>{shortCommit(run.commit)}</code> · <RunStatusBadge status={runStatus(run)} />
          {run.started && ` ${runDuration(run, now)}`}
        </span>
      </a>
      <RunLink run={run} provider={provider} className="dashboard-ci-github" />
    </li>
  );
}
