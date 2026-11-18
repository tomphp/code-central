// What a Spec row and a Lane header say of a Spec: its kind, its Stage
// progress for a Requirement, whether it needs the human and why, and its
// one-line headline. Its kind, first need and whether a Factory is on it
// are the Spec row's, from Go; this only words them.
import { STATES, type NeedsMeReason, type SpecKind, type StateSummary } from "../api/types";
import { REASON_LABEL } from "../format";
import type { ProjectSpec, ProjectTicket } from "../live";
import { mapCount } from "../map/mapCount";
import { headline, progress, type Progress } from "../requirement/progress";

// KindText is what a Spec row's badge says: nothing for a Requirement.
export type KindText = "Requirement" | "Map" | "Long-lived" | "Spec";

// KIND_TEXT is each Spec kind as its badge words it.
const KIND_TEXT: Record<SpecKind, KindText> = { requirement: "Requirement", map: "Map", long_lived: "Long-lived", spec: "Spec" };

// kindText is the badge of a Spec's kind; a Spec not loaded yet reads as
// a Spec.
export const kindText = (spec?: ProjectSpec): KindText => KIND_TEXT[spec?.kind ?? "spec"];

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
): { kind: KindText; progress?: Progress; headline: string } {
  const kind = kindText(spec);
  if (spec?.requirement) {
    const p = progress(spec.requirement, all, reasonOf);
    return { kind, progress: p, headline: headline(spec.finished, p) };
  }
  if (spec?.map) return { kind, headline: spec.finished ? "Finished" : mapCount(spec.map) };
  return { kind, headline: spec ? specHeadline(spec, summary) : "" };
}

function specHeadline({ finished, need, claimed }: ProjectSpec, summary: StateSummary): string {
  const done = summary.Done ?? 0;
  const total = STATES.reduce((n, s) => n + (summary[s] ?? 0), 0);
  if (finished) return `Finished · ${done} of ${total} Tickets Done`;
  const doing = need ? REASON_LABEL[need] : claimed ? "A Factory is on it" : undefined;
  return [doing, total > 0 && `${done} of ${total} Done`].filter(Boolean).join(" · ");
}

export function countStates(tickets: ProjectTicket[]): StateSummary {
  const counts: StateSummary = {};
  for (const t of tickets) counts[t.state] = (counts[t.state] ?? 0) + 1;
  return counts;
}
