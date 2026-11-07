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
import type { Api, EventHandlers, TicketAction, TicketActions } from "./api";
import { fakeActions } from "./fakeActions";
import {
  ApiError,
  FINISHED,
  type ChangeEvent,
  type FindTicketsFilter,
  type LinkedRow,
  type Mode,
  type NeedsMeReason,
  type NeedsMeRow,
  type NeedsMeView,
  type ProjectRow,
  type ProjectView,
  type SpecQuestions,
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
export type SeedRow = Partial<NeedsMeRow> & { id: string };

export class FakeApi implements Api {
  now = new Date("2026-11-07T09:00:00Z");
  // calls records each read and action by name, for tests that check a refetch.
  calls: string[] = [];
  private projects = new Map<string, FakeProject>();
  private tickets = new Map<string, TicketView>();
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
        return {
          project: this.projectOf(r.id),
          title: t?.title ?? r.id,
          state: t?.state ?? "In Progress",
          since: this.now.toISOString(),
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
    return [...this.tickets.values()]
      .filter((t) => this.projectOf(t.id) === project)
      .filter((t) => (filter.state?.length ? filter.state.includes(t.state) : !FINISHED.includes(t.state)))
      .map((t) => ({
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

  async getTicket(project: string, id: string): Promise<TicketView> {
    this.calls.push("getTicket");
    return this.view(this.ticket(project, id));
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
