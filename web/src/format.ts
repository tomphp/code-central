import type { CIRunView, NeedsMeReason, NeedsMeRow } from "./api/types";

export const REASON_LABEL: Record<NeedsMeReason, string> = {
  needs_answer: "Questions",
  answered: "Answered",
  silent_claim: "Silent Claim",
  long_claim: "Long Claim",
  failed: "Failed",
  in_review: "In Review",
  ci_failed: "CI failed",
  ci_cancelled: "CI cancelled",
  base_diverged: "Base diverged",
  base_missing: "Base missing",
  repository_missing: "Repository missing",
  pull_request_closed: "Pull request closed",
  to_merge: "To merge",
  ci_unavailable: "CI unavailable",
  accepted_not_landed: "Accepted but not landed",
  blocked_by_cancelled: "Blocked by a Cancelled Ticket",
  new_issue: "New issue",
  closed_externally: "Closed externally",
};

// shortCommit is a commit as the UI shows it: its first 7 characters.
export const shortCommit = (commit: string) => commit.slice(0, 7);

// runDuration is how long a CI Run took, or has taken so far: "45s",
// "3m 12s", "1h 5m"; "–" before it starts.
export function runDuration(run: Pick<CIRunView, "status" | "started" | "finished">, now: Date): string {
  if (!run.started) return "–";
  const end = run.status === "completed" && run.finished ? Date.parse(run.finished) : now.getTime();
  const s = Math.max(0, Math.round((end - Date.parse(run.started)) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ${s % 60}s`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

// ticketOf is the Ticket a Needs Me row opens on: its own, or the first
// a CI item lists (code-central-432); "" for a Project's item.
export function ticketOf(r: NeedsMeRow): string {
  return r.id || r.ci_item?.tickets[0]?.id || "";
}

// projectItemOf tells a Project's items of one reason apart: a CI item by
// its pushed commit, a Repository missing item by its Repository; "" for
// any other row.
export function projectItemOf(r: NeedsMeRow): string {
  return r.ci_item?.commit ?? r.repository_missing?.repository ?? "";
}

// listedTickets names the Tickets a CI item lists.
function listedTickets(r: NeedsMeRow): string {
  return (r.ci_item?.tickets ?? []).map((t) => t.id).join(", ");
}

// duration is how long ago `at` was, roughly: "just now", "5m", "3h", "2d".
export function duration(now: Date, at: string): string {
  const m = Math.floor((now.getTime() - Date.parse(at)) / 60_000);
  if (m < 1) return "just now";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

// rowDetail is a Needs Me row's one-line detail for its reason.
export function rowDetail(now: Date, r: NeedsMeRow): string {
  switch (r.reason) {
    case "needs_answer":
      return r.round
        ? `${r.round.open} of ${r.round.questions} open · Round ${r.round.number}`
        : "Questions waiting";
    case "answered":
      return `Answered ${ago(now, r.since)}, no session has picked it up`;
    case "silent_claim":
      return `Quiet for ${duration(now, r.since)}`;
    case "long_claim":
      return `No move for ${duration(now, r.since)}`;
    case "failed":
      if (!r.failure) return "Failed";
      return r.failure.reports > 1
        ? `${r.failure.headline} · failed ${r.failure.reports}×`
        : r.failure.headline;
    case "in_review":
      return r.summary || "Finished";
    case "ci_failed": {
      // One item per pushed commit, listing the Tickets waiting on it.
      const failed = r.ci_failure ? `${r.ci_failure.workflows.join(", ")} failed` : "CI failed";
      return r.ci_item ? `${listedTickets(r)} · ${failed}` : failed;
    }
    case "ci_cancelled": {
      const cancelled = `CI cancelled ${ago(now, r.since)}`;
      return r.ci_item ? `${listedTickets(r)} · ${cancelled}` : cancelled;
    }
    case "base_diverged":
      return r.base_diverged
        ? `${r.base_diverged.base ?? "The base"} is at ${shortCommit(r.base_diverged.local)} and the remote's at ${shortCommit(r.base_diverged.remote)}: reconcile them by hand`
        : "The local base has diverged from the remote's";
    case "base_missing":
      return r.base_missing?.fix ?? "The base branch doesn't exist";
    case "repository_missing":
      return r.repository_missing
        ? `${r.repository_missing.repository}: ${r.repository_missing.problem}`
        : "A Repository has no clone";
    case "pull_request_closed":
      // With several Repositories each closed one is named by its
      // Repository (Spec code-central-848).
      if (r.closed_pull_requests?.some((p) => p.repository)) {
        const named = r.closed_pull_requests.map((p) => `#${p.number} in ${p.repository}`).join(", ");
        return `Pull request ${named} ${r.closed_pull_requests.length === 1 ? "was" : "were"} closed without merging: send the work back or reopen it`;
      }
      return r.pull_request
        ? `Pull request #${r.pull_request.number} was closed without merging: send the work back or cancel it`
        : "Its pull request was closed without merging";
    case "to_merge":
      if (r.pull_request) {
        const conflicting =
          r.pull_request.state === "conflicting" ? ", conflicting" : "";
        return `Accepted ${ago(now, r.since)}: merge pull request #${r.pull_request.number}${conflicting}`;
      }
      return `Accepted ${ago(now, r.since)}: merge ${r.branch ?? "its branch"} into the base`;
    case "ci_unavailable":
      return ciUnavailableDetail(now, r);
    case "accepted_not_landed":
      return `Accepted ${ago(now, r.since)}, not landed`;
    case "blocked_by_cancelled":
      return r.blocker
        ? `Blocked by ${r.blocker.id} ${r.blocker.title}, Cancelled with no Replacement`
        : "Blocked by a Cancelled Ticket";
    case "new_issue":
      return r.link
        ? `${r.link.integration} ${r.link.id} came in ${ago(now, r.since)}`
        : `Came in ${ago(now, r.since)}`;
    case "closed_externally":
      return `${externalDetail(r)} · still ${r.state}`;
  }
}

// ciUnavailableDetail says why a CI unavailable row has had no word of
// its CI since its push.
function ciUnavailableDetail(now: Date, r: NeedsMeRow): string {
  const u = r.ci_unavailable;
  const listed = r.ci_item ? `${listedTickets(r)} · ` : "";
  if (!u) return `${listed}CI unavailable since ${ago(now, r.since)}`;
  const at = `${shortCommit(u.commit)} pushed ${ago(now, u.pushed)}`;
  if (u.cause === "no_ci_runs") return `${listed}No CI Runs reported for ${at}`;
  return u.message
    ? `${listed}CI Watcher not Watching: ${u.message} · ${at}`
    : `${listed}CI Watcher not Watching · ${at}`;
}

// externalDetail says what happened to a closed-externally row's external item.
function externalDetail(r: NeedsMeRow): string {
  const l = r.link;
  if (!l) return "Closed externally";
  switch (l.external_state) {
    case "deleted":
      return `Deleted in ${l.integration} ${l.id}`;
    case "out_of_scope":
      return `Left ${l.integration}'s query (${l.id})`;
    default:
      return `Closed in ${l.integration} ${l.id}`;
  }
}

export function ago(now: Date, at: string): string {
  const d = duration(now, at);
  return d === "just now" ? d : `${d} ago`;
}

// bytes is a size in readable units: bytes under 1 KB, then KB, then MB,
// to one decimal place (1 KB is 1024 bytes).
export function bytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${oneDecimal(n / 1024)} KB`;
  return `${oneDecimal(n / (1024 * 1024))} MB`;
}

// count is a whole number with thousands separators: "12,345".
export function count(n: number): string {
  return n.toLocaleString("en-US");
}

function oneDecimal(x: number): string {
  return String(Math.round(x * 10) / 10);
}

// CLAUDE_CODE_DEFAULT stands for a model or effort left unset, so the
// session ran on Claude Code's own default.
export const CLAUDE_CODE_DEFAULT = "Claude Code's default";
