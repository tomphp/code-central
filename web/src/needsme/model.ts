// The Needs Me read model: one NeedsMe built from the Inbox data on each
// load, which every view asks instead of working Needs Me out itself. Its
// answers are core's: a Ticket's reason is its find_tickets row's
// needs_me_reason, and the counts are needs_me's count and counts. It owns
// the per-view Stage rules: the Inbox shows a Requirement's Spec row in
// place of its Stage Tickets, and the Board's Needs Me column shows them
// as Ticket cards. It also scopes the data to a Project and groups every
// Spec's Tickets by Spec, so the views start from ready rows.
import type { NeedsMeGroup, NeedsMeReason } from "../api/types";
import { requirementList } from "../inbox/lists";
import { groupBySpec } from "../inbox/groupBySpec";
import type { InboxData, ProjectSpec, ProjectTicket } from "../live";
import { stageRank } from "../requirement/progress";

// PopupKind is which popup the Board opens for an ID.
export type PopupKind = "spec" | "focus" | "ticket";

// NeedsMeRows is Needs Me as the Inbox lists it.
export interface NeedsMeRows {
  // requirements is the Requirements one of whose Stage Tickets needs the
  // human, in the Requirements view's order: each stands in for its Stage
  // Tickets' rows.
  requirements: ProjectSpec[];
  // groups is the rest of Needs Me's rows by reason, in its order, with
  // no empty group.
  groups: NeedsMeGroup[];
}

// NeedsMe is the questions every view asks of Needs Me.
export interface NeedsMe {
  // reasonOf is why a Ticket needs the human, or a Spec: the first of its
  // Tickets that does, Stage Tickets first, in Stage order. None for an ID
  // that doesn't, or isn't loaded.
  reasonOf(id: string): NeedsMeReason | undefined;
  // count is the Needs Me count, of every Project or of one.
  count(project?: string): number;
  // rows is Needs Me in the Project, or in every Project, for the Inbox.
  rows(project?: string): NeedsMeRows;
  // needing is the Tickets in the Project that need the human, Stage
  // Tickets as themselves, in Needs Me's order: the Board's Needs Me column.
  needing(project?: string): ProjectTicket[];
  // tickets, requirements and maps are the Project's, or every Project's.
  tickets(project?: string): ProjectTicket[];
  requirements(project?: string): ProjectSpec[];
  maps(project?: string): ProjectSpec[];
  // ticketsOf is every one of a Spec's Tickets, in every State and Project.
  ticketsOf(spec: string): ProjectTicket[];
  // popupKind is the popup an ID opens: its Spec popup for a Spec, its
  // Focus popup while a Ticket needs the human, else its Ticket popup.
  // None for any other ID, or before the data loads.
  popupKind(id: string): PopupKind | undefined;
}

// CI_ITEM_REASONS are the reasons of a CI item: one row per commit,
// listing the Tickets waiting on it.
const CI_ITEM_REASONS: NeedsMeReason[] = ["ci_failed", "ci_cancelled", "ci_unavailable"];

// isCIItemReason reports whether a reason is a CI item's, which a listed
// Ticket's own pane leaves to the item.
export const isCIItemReason = (r: NeedsMeReason | undefined): boolean => !!r && CI_ITEM_REASONS.includes(r);

// needsMeOf builds the read model from one Inbox load; before the first,
// it is the model of no data, which needs nothing.
export function needsMeOf(data: InboxData | undefined): NeedsMe {
  const tickets = data?.tickets ?? [];
  const groups = data?.needsMe.groups ?? [];
  const inProject = (project?: string) => (x: { project: string }) => !project || x.project === project;
  const byId = new Map(tickets.map((t) => [t.id, t]));
  const bySpec = new Map(groupBySpec(tickets).map((g) => [g.id, g.tickets]));
  const specs = new Set((data?.specs ?? []).map((s) => s.id));
  const ticketsOf = (spec: string) => bySpec.get(spec) ?? [];
  // A Spec's reason is its Stage Tickets', in Stage order, then its other
  // Tickets' in list order, as progress orders a Requirement's needs.
  const reasonOf = (id: string): NeedsMeReason | undefined => {
    const t = byId.get(id);
    if (t) return t.needs_me_reason;
    return [...ticketsOf(id)].sort((a, b) => stageRank(a) - stageRank(b)).find((x) => x.needs_me_reason)?.needs_me_reason;
  };
  // order is where each Ticket comes in Needs Me, a CI item's Tickets at
  // the item's place.
  const order = new Map<string, number>();
  for (const r of groups.flatMap((g) => g.tickets)) {
    for (const id of r.id ? [r.id] : (r.ci_item?.tickets ?? []).map((t) => t.id)) if (!order.has(id)) order.set(id, order.size);
  }
  const place = (t: ProjectTicket) => order.get(t.id) ?? order.size;

  return {
    reasonOf,
    count: (project) => (project ? (data?.needsMe.counts[project] ?? 0) : (data?.needsMe.count ?? 0)),
    rows: (project) => {
      const shown = groups.map((g) => ({ ...g, tickets: g.tickets.filter(inProject(project)) }));
      const staged = new Set(shown.flatMap((g) => g.tickets).flatMap((r) => (r.requirement?.stage ? [r.requirement.spec] : [])));
      return {
        requirements: requirementList((data?.requirements ?? []).filter(inProject(project))).filter((s) => staged.has(s.id)),
        groups: shown.map((g) => ({ ...g, tickets: g.tickets.filter((r) => !r.requirement?.stage) })).filter((g) => g.tickets.length > 0),
      };
    },
    needing: (project) =>
      tickets
        .filter(inProject(project))
        .filter((t) => t.needs_me_reason)
        .sort((a, b) => place(a) - place(b)),
    tickets: (project) => tickets.filter(inProject(project)),
    requirements: (project) => (data?.requirements ?? []).filter(inProject(project)),
    maps: (project) => (data?.maps ?? []).filter(inProject(project)),
    ticketsOf,
    popupKind: (id) => {
      if (!data) return undefined;
      if (specs.has(id)) return "spec";
      if (!byId.has(id)) return undefined;
      return reasonOf(id) ? "focus" : "ticket";
    },
  };
}
