// Auto-accept as the UI shows it (Spec code-central-536): the names of the
// Auto-accept kinds and the line a Move that an auto-accept made reads as.
import { MAP_LABEL, type AutoAcceptCause, type AutoAcceptKind, type MoveView, type TicketView } from "../api/types";
import { stageOf } from "../requirement/progress";

export const AUTO_ACCEPT_KIND_NAME: Record<AutoAcceptKind, string> = {
  grilling: "Grilling outcomes",
  spec: "Spec recommendation",
  tickets: "Tickets Stage",
  implementation: "Implementation results",
  merge: "Merge results",
};

// autoAcceptKindOf is the kind of a Ticket as core works it out on read: a
// Stage's own, merge on a Spec merge Ticket, else Implementation results.
// (A Prototype or Map Ticket has none and is never auto-accepted.)
function autoAcceptKindOf(t: Pick<TicketView, "labels" | "spec_merge_ticket">): AutoAcceptKind {
  return stageOf(t) ?? (t.spec_merge_ticket ? "merge" : "implementation");
}

export const AUTO_ACCEPT_LABEL = "auto-accept";

// canAutoAccept mirrors core's autoAcceptKind: a Prototype, a Wayfinder-type
// Ticket and a Map's Ticket are never auto-accepted. specLabels are the
// Labels of the Ticket's Spec.
export function canAutoAccept(t: { labels: string[] }, specLabels: string[]): boolean {
  return !t.labels.some((l) => l === "prototype" || l.startsWith("wayfinder:")) && !specLabels.includes(MAP_LABEL);
}

// wouldAutoAccept is core's autoAcceptCause for the Label case, for a
// Ticket In Review: canAutoAccept, and a spec Stage needs its Spec
// recommendation.
export function wouldAutoAccept(t: Pick<TicketView, "labels" | "recommendation">, specLabels: string[]): boolean {
  return canAutoAccept(t, specLabels) && (stageOf(t) !== "spec" || !!t.recommendation);
}

function causeText(cause: AutoAcceptCause, t: Pick<TicketView, "labels" | "spec" | "spec_merge_ticket">): string {
  switch (cause) {
    case "project":
      return `Project setting: ${AUTO_ACCEPT_KIND_NAME[autoAcceptKindOf(t)]}`;
    case "ticket-label":
      return "auto-accept Label";
    case "spec-label":
      return `auto-accept Label on ${t.spec.title}`;
  }
}

// moveText is a Move's history line: "In Review → Accepted by human", and
// for an auto-accept "…, Auto-accepted (Project setting: Implementation
// results)".
export function moveText(m: MoveView, t: Pick<TicketView, "labels" | "spec" | "spec_merge_ticket">): string {
  const text = `${m.from ? `${m.from} → ` : ""}${m.to} by ${m.actor}`;
  return m.cause ? `${text}, Auto-accepted (${causeText(m.cause, t)})` : text;
}
