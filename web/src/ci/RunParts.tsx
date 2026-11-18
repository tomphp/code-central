// The shared CI Run parts (Spec code-central-650): a run's status badge,
// its ignored mark and its link to the CI system, which every CI Run
// display composes through CIRunEntry.
import type { CIConclusion, CIProvider, CIRunView } from "../api/types";

// CONCLUSION_LABEL says how a finished run or job ended.
export const CONCLUSION_LABEL: Record<CIConclusion, string> = {
  success: "passed",
  failure: "failed",
  cancelled: "cancelled",
  timed_out: "timed out",
  skipped: "skipped",
  neutral: "neutral",
  partially_succeeded: "partially succeeded",
};

// runStatus is a run's status as a CI Run display says it.
export function runStatus(run: Pick<CIRunView, "status" | "conclusion">): string {
  if (run.status === "queued") return "queued";
  if (run.status === "in_progress" || !run.conclusion) return "running";
  return CONCLUSION_LABEL[run.conclusion];
}

// runName is the name a run shows: its workflow's display name, or its file.
export const runName = (run: CIRunView) => run.name || run.workflow;

// statusClass is the CSS class of a status label, such as "ci-timed-out".
const statusClass = (status: string) => `ci-${status.toLowerCase().replace(" ", "-")}`;

// RunStatusBadge is a status as a badge. A partially succeeded one has the
// passed styling with a dashed border.
export function RunStatusBadge({ status }: { status: string }) {
  return <span className={`ci-status ${statusClass(status)}`}>{status}</span>;
}

// IgnoredMark marks a run of an ignored workflow; the entry holding it
// carries the ignored class, which greys it.
export function IgnoredMark() {
  return <span className="label">ignored</span>;
}

// RunLink opens a run on its CI system, named from the Project's CI
// Provider, in a new tab.
export function RunLink({ run, provider, className }: { run: Pick<CIRunView, "url">; provider?: CIProvider; className?: string }) {
  return (
    <a href={run.url} target="_blank" rel="noreferrer" className={className}>
      {provider === "azure_pipelines" ? "View on Azure DevOps" : "View on GitHub"}
    </a>
  );
}
