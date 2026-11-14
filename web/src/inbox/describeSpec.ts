// What a Spec row and a Lane header say of a Spec: its kind, its Stage
// progress for a Requirement, whether it needs the human and why, and its
// one-line headline.
import { LONG_LIVED_LABEL, STATES, type NeedsMeReason, type State, type StateSummary } from "../api/types";
import { REASON_LABEL } from "../format";
import type { ProjectSpec, ProjectTicket } from "../live";
import { mapCount } from "../map/mapCount";
import { headline, progress, type Progress } from "../requirement/progress";

// SpecKind is what a Spec row's badge says: nothing for a Requirement.
export type SpecKind = "Requirement" | "Map" | "Long-lived" | "Spec";

export const specKindOf = (spec?: ProjectSpec): SpecKind =>
  spec?.requirement ? "Requirement" : spec?.map ? "Map" : spec?.labels.includes(LONG_LIVED_LABEL) ? "Long-lived" : "Spec";

// describeSpec is what a Spec row says of its Spec, by kind; whether it
// needs the human is the Needs Me read model's to say. A Requirement has
// its Stage progress and today's headline. A Map's headline is its
// decision count. Any other Spec's headline is its first need, else
// whether a Factory is on it, then how many of its Tickets are Done.
export function describeSpec(
  spec: ProjectSpec | undefined,
  all: ProjectTicket[],
  summary: StateSummary,
  reasonOf: (id: string) => NeedsMeReason | undefined,
): { kind: SpecKind; progress?: Progress; headline: string } {
  if (spec?.requirement) {
    const p = progress(spec.requirement, all, reasonOf);
    return { kind: "Requirement", progress: p, headline: headline(spec.finished, p) };
  }
  if (spec?.map) return { kind: "Map", headline: spec.finished ? "Finished" : mapCount(spec.map) };
  return { kind: specKindOf(spec), headline: spec ? specHeadline(spec.finished, firstNeed(all, reasonOf), all, summary) : "" };
}

function specHeadline(finished: boolean, need: NeedsMeReason | undefined, all: ProjectTicket[], summary: StateSummary): string {
  const done = summary.Done ?? 0;
  const total = STATES.reduce((n, s) => n + (summary[s] ?? 0), 0);
  if (finished) return `Finished · ${done} of ${total} Tickets Done`;
  const doing = need ? REASON_LABEL[need] : all.some((t) => CLAIMED.includes(t.state)) ? "A Factory is on it" : undefined;
  return [doing, total > 0 && `${done} of ${total} Done`].filter(Boolean).join(" · ");
}

// CLAIMED is the States a Ticket holds a live Claim in.
const CLAIMED: State[] = ["In Progress", "Needs Answer", "Answered"];

// firstNeed is the reason of the first of the Tickets that needs the human.
function firstNeed(tickets: ProjectTicket[], reasonOf: (id: string) => NeedsMeReason | undefined): NeedsMeReason | undefined {
  for (const t of tickets) {
    const r = reasonOf(t.id);
    if (r) return r;
  }
  return undefined;
}

export function countStates(tickets: ProjectTicket[]): StateSummary {
  const counts: StateSummary = {};
  for (const t of tickets) counts[t.state] = (counts[t.state] ?? 0) + 1;
  return counts;
}
