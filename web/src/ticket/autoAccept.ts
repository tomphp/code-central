// Auto-accept as the UI shows it (Spec code-central-536): the names of the
// Auto-accept kinds and the line a Move that an auto-accept made reads as.
import type { AutoAcceptCause, AutoAcceptKind, MoveView, TicketView } from "../api/types";
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
