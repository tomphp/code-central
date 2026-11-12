// groupBySpec groups a State list's Tickets (already in list order) into one
// card per Spec: a Spec takes the place of its first Ticket, and its
// Tickets keep their order inside it.
import { STATES, type NeedsMeReason, type State } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";

export type SpecKind = "Requirement" | "Map" | "Spec";

export const specKindOf = (spec?: ProjectSpec): SpecKind => (spec?.requirement ? "Requirement" : spec?.map ? "Map" : "Spec");

export interface SpecCard {
  id: string;
  // title is the Spec's ID while its Spec isn't loaded yet.
  title: string;
  kind: SpecKind;
  tickets: ProjectTicket[];
  // total is all the Spec's Tickets; 0 while its Spec isn't loaded.
  total: number;
  // counts is the Spec's Tickets per State, in STATES order, non-zero only.
  counts: { state: State; count: number }[];
  needsYou: boolean;
}

export function groupBySpec(tickets: ProjectTicket[], specs: Map<string, ProjectSpec>, reasons: Map<string, NeedsMeReason>): SpecCard[] {
  const cards = new Map<string, SpecCard>();
  for (const t of tickets) {
    let card = cards.get(t.spec);
    if (!card) {
      const spec = specs.get(t.spec);
      const summary = spec?.state_summary ?? {};
      const counts = STATES.flatMap((state) => (summary[state] ? [{ state, count: summary[state]! }] : []));
      card = {
        id: t.spec,
        title: spec?.title ?? t.spec,
        kind: specKindOf(spec),
        tickets: [],
        total: counts.reduce((n, c) => n + c.count, 0),
        counts,
        needsYou: false,
      };
      cards.set(t.spec, card);
    }
    card.tickets.push(t);
    card.needsYou ||= reasons.has(t.id);
  }
  return [...cards.values()];
}
