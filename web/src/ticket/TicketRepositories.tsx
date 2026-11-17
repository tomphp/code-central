// A Ticket's Repositories section (Spec code-central-848): one row per
// Changed Repository, in landing order, with its branch@commit, its landed
// commit (or not landed and why, while the Ticket waits to land), its pull request
// badge and its CI result with the pushed commit it comes from. While the
// Ticket is In Progress the rows are the Factory's live report, which its
// finish settles. Reads carry the Changed Repositories only while the
// Project has more than one Repository, so a single-repository Project's
// Ticket has no such section.
import type { ChangedRepositoryView, TicketView } from "../api/types";
import { shortCommit } from "../format";
import { PullRequestBadge } from "./PullRequestBadge";

export function TicketRepositories({ ticket: t }: { ticket: Pick<TicketView, "state" | "repositories"> }) {
  const rs = t.repositories ?? [];
  if (rs.length === 0) return null;
  return (
    <section aria-label="Repositories" className="repos-section">
      <h3>
        Repositories{t.state === "In Progress" && <span className="muted small"> changed so far</span>}
      </h3>
      <ul className="repos">
        {rs.map((r) => (
          <li key={r.name} aria-label={r.name}>
            <span className="repo-name">{r.name}</span> <Work r={r} />
            <Landing r={r} waiting={t.state === "Accepted"} />
            <PullRequestBadge pr={r.pull_request} />
            <CI r={r} />
          </li>
        ))}
      </ul>
    </section>
  );
}

// Work is the Repository's branch@commit, or whichever of them it has.
function Work({ r }: { r: ChangedRepositoryView }) {
  const commit = r.commit && shortCommit(r.commit);
  const text = r.branch && commit ? `${r.branch}@${commit}` : (r.branch ?? commit);
  return text ? <code>{text}</code> : null;
}

// Landing is the commit the work landed at, or not landed, with why, while
// the Ticket waits to land.
function Landing({ r, waiting }: { r: ChangedRepositoryView; waiting: boolean }) {
  if (r.landed) {
    return (
      <span className="small muted">
        {" "}
        · landed at <code>{shortCommit(r.landed.commit)}</code>
      </span>
    );
  }
  if (!waiting) return null;
  return <span className="small muted"> · not landed{r.not_landed && `: ${r.not_landed}`}</span>;
}

// CI is the Repository's CI result with its pushed commit, else the
// latest commit pushed for it; or that CI Tracking doesn't watch it.
function CI({ r }: { r: ChangedRepositoryView }) {
  if (r.ci_not_watched) return <span className="small muted"> · CI not watched</span>;
  const pushed = r.ci_commit ?? r.pushed_commit?.commit;
  if (!r.ci_result && !pushed) return null;
  return (
    <span className="small">
      {" "}
      {r.ci_result ? <span className={`ci-result ci-${r.ci_result}`}>CI {r.ci_result}</span> : "pushed"}
      {pushed && (
        <>
          {" "}
          <code>{shortCommit(pushed)}</code>
        </>
      )}
    </span>
  );
}
