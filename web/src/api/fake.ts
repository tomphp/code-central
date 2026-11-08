// FakeApi is the in-memory fake of the API module, for component and
// layout tests. The contract suite (contract.ts) runs against it and the
// real module alike.
//
// It holds data, not rules: a test seeds Projects and Tickets and says
// what Needs Me returns (setNeedsMe), in the order the service would send
// it. Project counts and each Ticket's needs_me_reason are read from that.
// Mutators send the notice the service would (addProject:
// project.changed, addTicket and updateTicket: ticket.changed);
// setNeedsMe sends none, so a test that changes it calls emit itself.
//
// A few rules are the contract's and so the fake's too: findTickets
// leaves finished Tickets out unless asked, and an Archived Project
// (archiveProject, refused while any Ticket is unfinished) refuses every
// action until unarchiveProject.
//
// Specs are seeded with addSpec, or exist because a Ticket names them.
// A Spec read derives what the service derives from its Tickets: the
// State summary, finished, and a Requirement's requirement block
// (a Stage's Needs Me reason is Needs Me's, as set by setNeedsMe).
// submitRequirement writes a Spec and its three Stage Tickets.
import { numberOf, type Api, type EventHandlers, type TicketAction, type TicketActions } from "./api";
import { fakeActions } from "./fakeActions";
import {
  ApiError,
  FINISHED,
  type ChangeEvent,
  type FindSpecsFilter,
  type FindTicketsFilter,
  type LinkedRow,
  type Mode,
  type NeedsMeReason,
  type NeedsMeRow,
  type NeedsMeView,
  type ProjectRow,
  type ProjectView,
  type RequirementView,
  type SpecDetail,
  type SpecQuestions,
  type SpecRow,
  type SpecView,
  type StageName,
  type StateSummary,
  type SubmitRequirementResult,
  type TicketRow,
  type TicketView,
} from "./types";

// Every action's fake behaviour registers itself from ./actions/.
import.meta.glob("./actions/*.ts", { eager: true });

interface FakeProject {
  key: string;
  display_name: string;
  created: string;
  mode: Mode;
  archived: boolean;
  archived_at?: string;
  next: number;
}

export type SeedTicket = Partial<TicketView> & { title: string };
export type SeedSpec = Partial<SpecView> & { title: string };

// The Stages in order, with the title each Stage Ticket's starts with.
const STAGES: { name: StageName; title: string }[] = [
  { name: "grilling", title: "Grilling" },
  { name: "spec", title: "Spec" },
  { name: "tickets", title: "Tickets" },
];
const stageOf = (t: { labels: string[] }) => STAGES.find((s) => t.labels.includes(`stage:${s.name}`))?.name;
export type SeedRow = Partial<NeedsMeRow> & { id: string };

export class FakeApi implements Api {
  now = new Date("2026-11-07T09:00:00Z");
  // calls records each read and action by name, for tests that check a refetch.
  calls: string[] = [];
  private projects = new Map<string, FakeProject>();
  private tickets = new Map<string, TicketView>();
  private specs = new Map<string, SpecView>();
  private groups: { reason: NeedsMeReason; tickets: NeedsMeRow[] }[] = [];
  private subs = new Set<EventHandlers>();

  // ---- seeding ----

  addProject(key: string, display_name = key, opts: { archived?: boolean } = {}) {
    this.projects.set(key, {
      key,
      display_name,
      created: this.now.toISOString(),
      mode: "committed",
      archived: opts.archived ?? false,
      ...(opts.archived ? { archived_at: this.now.toISOString() } : {}),
      next: 1,
    });
    this.emit({ type: "project.changed", project: key });
  }

  addTicket(project: string, seed: SeedTicket): TicketView {
    const p = this.projects.get(project);
    if (!p) throw new Error(`FakeApi: no Project ${project}`);
    const at = this.now.toISOString();
    const t: TicketView = {
      id: `${project}-${p.next++}`,
      state: "Draft",
      spec: { id: "", title: "" },
      labels: [],
      tags: {},
      blocked_by: [],
      blockers: [],
      replacements: [],
      created: at,
      updated: at,
      description: "",
      claims: [],
      rounds: [],
      failure_reports: [],
      comments: [],
      moves: [],
      edits: [],
      blocked: false,
      claimable: false,
      ...seed,
    };
    if (seed.id) p.next = Math.max(p.next, Number(seed.id.slice(project.length + 1)) + 1);
    this.tickets.set(t.id, t);
    this.emit({ type: "ticket.changed", project, id: t.id });
    return structuredClone(t);
  }

  // addSpec seeds a Spec; its ID comes from the Project's one counter,
  // shared with Tickets, as the service's does.
  addSpec(project: string, seed: SeedSpec): SpecView {
    const p = this.projects.get(project);
    if (!p) throw new Error(`FakeApi: no Project ${project}`);
    const at = this.now.toISOString();
    const s: SpecView = { id: `${project}-${p.next++}`, labels: [], tags: {}, created: at, updated: at, body: "", edits: [], ...seed };
    if (seed.id) p.next = Math.max(p.next, Number(seed.id.slice(project.length + 1)) + 1);
    this.specs.set(s.id, s);
    this.emit({ type: "spec.changed", project, id: s.id });
    return structuredClone(s);
  }

  updateTicket(id: string, patch: Partial<TicketView>): TicketView {
    const t = this.tickets.get(id);
    if (!t) throw new Error(`FakeApi: no Ticket ${id}`);
    Object.assign(t, patch, { updated: this.now.toISOString() });
    this.emit({ type: "ticket.changed", project: this.projectOf(id), id });
    return this.view(t);
  }

  // setNeedsMe sets what needs_me returns, group by group in the order
  // given. A row's project, title and state come from its Ticket unless
  // given; since defaults to now.
  setNeedsMe(groups: { reason: NeedsMeReason; tickets: SeedRow[] }[]) {
    this.groups = groups.map((g) => ({
      reason: g.reason,
      tickets: g.tickets.map((r) => {
        const t = this.tickets.get(r.id);
        const requirement = t && this.requirementOf(t);
        return {
          project: this.projectOf(r.id),
          title: t?.title ?? r.id,
          state: t?.state ?? "In Progress",
          since: this.now.toISOString(),
          ...(requirement && { requirement }),
          ...r,
          reason: g.reason,
        };
      }),
    }));
  }

  // ---- for actions' fake behaviour (src/api/actions/) ----

  // needsMeGroups is what setNeedsMe last set, as full rows, for fake
  // actions that change Needs Me (read it, change it, set it back).
  get needsMeGroups(): { reason: NeedsMeReason; tickets: NeedsMeRow[] }[] {
    return structuredClone(this.groups);
  }

  // ticketsIn is every Ticket in a Project, as views.
  ticketsIn(project: string): TicketView[] {
    return [...this.tickets.values()].filter((t) => this.projectOf(t.id) === project).map((t) => this.view(t));
  }

  // dropNeedsMe takes a Ticket out of Needs Me, as an action that resolves
  // its reason would. It sends no notice (the action's update does).
  dropNeedsMe(id: string) {
    for (const g of this.groups) g.tickets = g.tickets.filter((r) => r.id !== id);
  }

  // ---- the live feed ----

  emit(e: ChangeEvent) {
    for (const h of this.subs) h.onEvent(e);
  }

  // reconnect stands in for the feed dropping and coming back.
  reconnect() {
    for (const h of this.subs) h.onOpen?.({ reconnect: true });
  }

  get subscribers() {
    return this.subs.size;
  }

  subscribe(h: EventHandlers): () => void {
    this.subs.add(h);
    queueMicrotask(() => this.subs.has(h) && h.onOpen?.({ reconnect: false }));
    return () => this.subs.delete(h);
  }

  // ---- reads ----

  async listProjects(): Promise<ProjectRow[]> {
    this.calls.push("listProjects");
    return [...this.projects.values()]
      .sort((a, b) => a.key.localeCompare(b.key))
      .map(({ next: _, ...p }) => ({
        ...p,
        needs_me: this.rows().filter((r) => r.project === p.key).length,
        unfinished: this.unfinished(p.key).length,
      }));
  }

  async needsMe(): Promise<NeedsMeView> {
    this.calls.push("needsMe");
    return structuredClone({ count: this.rows().length, groups: this.groups.filter((g) => g.tickets.length) });
  }

  async findTickets(project: string, filter: FindTicketsFilter = {}): Promise<TicketRow[]> {
    this.calls.push("findTickets");
    this.project(project);
    const rows = [...this.tickets.values()]
      .filter((t) => this.projectOf(t.id) === project)
      .filter((t) => (filter.state?.length ? filter.state.includes(t.state) : !FINISHED.includes(t.state)));
    return this.rowsOf(rows);
  }

  async getTicket(project: string, id: string): Promise<TicketView> {
    this.calls.push("getTicket");
    return this.view(this.ticket(project, id));
  }

  async getSpec(project: string, spec: string): Promise<SpecDetail> {
    this.calls.push("getSpec");
    this.project(project);
    const id = `${project}-${spec.slice(spec.lastIndexOf("-") + 1)}`;
    const s = this.allSpecs(project).find((s) => s.id === id);
    if (!s) throw new ApiError(404, "not_found", `there is no Spec ${id}`);
    const { state_summary, finished, requirement } = this.derive(id);
    return structuredClone({ ...s, state_summary, finished, tickets: this.rowsOf(this.ticketsOf(id)), ...(requirement && { requirement }) });
  }

  async findSpecs(project: string, filter: FindSpecsFilter = {}): Promise<SpecRow[]> {
    this.calls.push("findSpecs");
    this.project(project);
    return structuredClone(
      this.allSpecs(project)
        .map((s) => ({ s, ...this.derive(s.id) }))
        .filter((r) => (filter.finished || !r.finished) && (!filter.requirements || r.requirement))
        .map(({ s, state_summary, finished, requirement }) => ({
          id: s.id,
          title: s.title,
          labels: s.labels,
          tags: s.tags,
          state_summary,
          finished,
          updated: s.updated,
          ...(s.link && { link: s.link }),
          ...(requirement && { requirement }),
        })),
    );
  }

  async submitRequirement(project: string, text: string): Promise<SubmitRequirementResult> {
    this.calls.push("submitRequirement");
    this.writable(project);
    const title = text.trim().split("\n")[0].trim();
    if (!title) throw new ApiError(422, "invalid", "a Requirement needs text; its first line becomes the Spec's title", "text");
    const spec = this.addSpec(project, { title, body: text });
    const stages: TicketView[] = [];
    for (const s of STAGES) {
      const prev = stages.at(-1);
      stages.push(
        this.addTicket(project, {
          title: `${s.title}: ${title}`,
          state: "Ready",
          spec: { id: spec.id, title },
          labels: [`stage:${s.name}`],
          description: `The ${s.title} Stage of [${spec.id}](${spec.id}).\n`,
          blocked_by: prev ? [prev.id] : [],
          blockers: prev ? [{ id: prev.id, title: prev.title, state: prev.state }] : [],
          blocked: !!prev,
          claimable: !prev,
          moves: [{ to: "Ready", actor: "human", time: this.now.toISOString(), via: "submit_requirement" }],
        }),
      );
    }
    return { spec, stages };
  }

  // specQuestions reads a Spec from its seeded Tickets' spec, so a Spec
  // no Ticket names is not found.
  async specQuestions(project: string, spec: string): Promise<SpecQuestions> {
    this.calls.push("specQuestions");
    this.project(project);
    const id = `${project}-${spec.slice(spec.lastIndexOf("-") + 1)}`;
    const of = [...this.tickets.values()].filter((t) => t.spec.id === id);
    if (!of.length) throw new ApiError(404, "not_found", `there is no Spec ${id}`);
    return structuredClone({
      spec: id,
      tickets: of.filter((t) => t.rounds.length).map(({ id, title, state, rounds }) => ({ id, title, state, rounds })),
    });
  }

  async act<A extends TicketAction>(
    project: string,
    id: string,
    action: A,
    body: TicketActions[A]["body"],
  ): Promise<TicketActions[A]["result"]> {
    this.calls.push(action);
    const t = this.view(this.ticket(project, id));
    this.writable(project);
    const handler = fakeActions[action];
    if (!handler) throw new Error(`FakeApi: no fake behaviour for ${action}: add one under src/api/actions/`);
    return structuredClone(handler(this, t, body));
  }

  async archiveProject(key: string): Promise<ProjectView> {
    this.calls.push("archiveProject");
    const p = this.writable(key);
    const open = this.unfinished(key);
    if (open.length) {
      throw new ApiError(409, "action_not_allowed", `Project ${key} still has unfinished work: ${open.map((t) => `${t.id} (${t.state})`).join(", ")}`);
    }
    Object.assign(p, { archived: true, archived_at: this.now.toISOString() });
    this.emit({ type: "project.changed", project: key });
    return this.projectView(p);
  }

  async unarchiveProject(key: string): Promise<ProjectView> {
    this.calls.push("unarchiveProject");
    const p = this.project(key);
    if (p.archived) {
      p.archived = false;
      delete p.archived_at;
      this.emit({ type: "project.changed", project: key });
    }
    return this.projectView(p);
  }

  async renameProject(key: string, displayName: string): Promise<ProjectView> {
    this.calls.push("renameProject");
    const name = displayName.trim();
    if (!name) throw new ApiError(422, "invalid", "a Project needs a display name", "display_name");
    const p = this.writable(key);
    if (p.display_name !== name) {
      p.display_name = name;
      this.emit({ type: "project.changed", project: key });
    }
    return this.projectView(p);
  }

  // unlink detaches a seeded Ticket's Link, taking it out of Needs Me if it
  // was there as closed externally (the only reason a Link gives).
  async unlink(project: string, target: string): Promise<LinkedRow> {
    this.calls.push("unlink");
    const t = this.ticket(project, target);
    this.writable(project);
    if (!t.link) throw new ApiError(422, "invalid", `${t.id} is not linked`, "target");
    if (this.rows().find((r) => r.id === t.id)?.reason === "closed_externally") this.dropNeedsMe(t.id);
    const v = this.updateTicket(t.id, { link: { ...t.link, detached: true } });
    return { kind: "ticket", id: v.id, title: v.title, state: v.state, link: v.link! };
  }

  // ---- internals ----

  // allSpecs is a Project's seeded Specs, then any Spec only a Ticket
  // names, in ID order.
  private allSpecs(project: string): SpecView[] {
    const out = new Map([...this.specs].filter(([id]) => this.projectOf(id) === project));
    for (const t of this.tickets.values()) {
      if (this.projectOf(t.id) !== project || !t.spec.id || out.has(t.spec.id)) continue;
      out.set(t.spec.id, { id: t.spec.id, title: t.spec.title, labels: [], tags: {}, created: t.created, updated: t.updated, body: "", edits: [] });
    }
    return [...out.values()].sort((a, b) => Number(numberOf(a.id)) - Number(numberOf(b.id)));
  }

  private ticketsOf(spec: string): TicketView[] {
    return [...this.tickets.values()].filter((t) => t.spec.id === spec).sort((a, b) => Number(numberOf(a.id)) - Number(numberOf(b.id)));
  }

  private rowsOf(tickets: TicketView[]): TicketRow[] {
    return tickets.map((t) => ({
      id: t.id,
      title: t.title,
      state: t.state,
      spec: t.spec.id,
      labels: [...t.labels],
      blocked: t.blocked,
      updated: t.updated,
      ...(t.link && { link: { ...t.link } }),
    }));
  }

  // requirementOf is a Needs Me row's requirement field: set when the
  // Ticket's Spec is a Requirement, with stage only on a Stage Ticket.
  private requirementOf(t: TicketView): NeedsMeRow["requirement"] {
    if (!this.ticketsOf(t.spec.id).some((s) => stageOf(s))) return undefined;
    const stage = stageOf(t);
    return { spec: t.spec.id, title: this.specs.get(t.spec.id)?.title ?? t.spec.title, ...(stage && { stage }) };
  }

  // derive is what a Spec read works out from the Spec's Tickets.
  private derive(spec: string): { state_summary: StateSummary; finished: boolean; requirement?: RequirementView } {
    const tickets = this.ticketsOf(spec);
    const state_summary: StateSummary = {};
    for (const t of tickets) state_summary[t.state] = (state_summary[t.state] ?? 0) + 1;
    const finished = tickets.length > 0 && tickets.every((t) => FINISHED.includes(t.state));
    const stages = STAGES.map((s) => tickets.find((t) => stageOf(t) === s.name)).filter((t) => !!t);
    if (!stages.length) return { state_summary, finished };
    const reason = (t: TicketView) => this.rows().find((r) => r.id === t.id)?.reason;
    // A Ticket a Factory could claim now; one waiting on the human doesn't count.
    const claimable = (t: TicketView) =>
      t.state === "Ready" && !reason(t) && t.blocked_by.every((b) => this.tickets.get(b)?.state === "Done");
    const build = tickets.filter((t) => !stageOf(t) && t.state !== "Cancelled");
    const current = stages.find((t) => t.state !== "Done");
    const held = tickets.some((t) => t.claims.some((c) => !c.ended));
    return {
      state_summary,
      finished,
      requirement: {
        stages: stages.map((t) => ({ name: stageOf(t)!, ticket: t.id, state: t.state, ...(reason(t) && { needs_me_reason: reason(t) }) })),
        build: { done: build.filter((t) => t.state === "Done").length, total: build.length },
        waiting_for_factory: current ? claimable(current) : build.some(claimable) && !held,
      },
    };
  }

  private unfinished(key: string) {
    return [...this.tickets.values()].filter((t) => this.projectOf(t.id) === key && !FINISHED.includes(t.state));
  }

  private writable(key: string) {
    const p = this.project(key);
    if (p.archived) {
      throw new ApiError(409, "action_not_allowed", `Project ${key} is archived, so it takes no writes`, undefined, ["unarchive_project"]);
    }
    return p;
  }

  private projectView({ next: _, ...p }: FakeProject): ProjectView {
    return structuredClone(p);
  }

  private rows() {
    return this.groups.flatMap((g) => g.tickets);
  }

  private projectOf(id: string) {
    return id.slice(0, id.lastIndexOf("-"));
  }

  private project(key: string) {
    const p = this.projects.get(key);
    if (!p) throw new ApiError(404, "unknown_project", `unknown Project "${key}"`);
    return p;
  }

  private ticket(project: string, id: string) {
    this.project(project);
    const n = id.slice(id.lastIndexOf("-") + 1);
    const t = this.tickets.get(`${project}-${n}`);
    if (!t) throw new ApiError(404, "not_found", `no Ticket ${project}-${n}`);
    return t;
  }

  private view(t: TicketView): TicketView {
    const reason = this.rows().find((r) => r.id === t.id)?.reason;
    const v = structuredClone(t);
    if (reason) v.needs_me_reason = reason;
    else delete v.needs_me_reason;
    return v;
  }
}
