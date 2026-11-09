import type { NeedsMeReason, NeedsMeRow } from "./api/types";

export const REASON_LABEL: Record<NeedsMeReason, string> = {
  needs_answer: "Questions",
  answered: "Answered",
  silent_claim: "Silent Claim",
  long_claim: "Long Claim",
  failed: "Failed",
  in_review: "In Review",
  to_merge: "To merge",
  accepted_not_landed: "Accepted but not landed",
  blocked_by_cancelled: "Blocked by a Cancelled Ticket",
  closed_externally: "Closed externally",
};

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
      return r.round ? `${r.round.open} of ${r.round.questions} open · Round ${r.round.number}` : "Questions waiting";
    case "answered":
      return `Answered ${ago(now, r.since)}, no session has picked it up`;
    case "silent_claim":
      return `Quiet for ${duration(now, r.since)}`;
    case "long_claim":
      return `No move for ${duration(now, r.since)}`;
    case "failed":
      if (!r.failure) return "Failed";
      return r.failure.reports > 1 ? `${r.failure.headline} · failed ${r.failure.reports}×` : r.failure.headline;
    case "in_review":
      return r.summary || "Finished";
    case "to_merge":
      return r.branch ? `Accepted ${ago(now, r.since)}: merge ${r.branch} into the base` : `Accepted ${ago(now, r.since)}: merge its branch into the base`;
    case "accepted_not_landed":
      return `Accepted ${ago(now, r.since)}, not landed`;
    case "blocked_by_cancelled":
      return r.blocker ? `Blocked by ${r.blocker.id} ${r.blocker.title}, Cancelled with no Replacement` : "Blocked by a Cancelled Ticket";
    case "closed_externally":
      return `${externalDetail(r)} · still ${r.state}`;
  }
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
