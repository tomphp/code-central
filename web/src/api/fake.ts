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
//
// recordCall stands in for an agent's MCP call, which the service records
// as Usage itself; usage reads the calls back as the service does: over
// the last days local days, for one Project or all of them.
//
// The Options start at their defaults; setOptions validates as the
// service does, and sends options.changed only when something changed.
//
// postSessionEvents stands in for a Factory's POST /api/session-events: the
// events go to the newest Claim with that session ID. Each Claim view
// carries its latest Session Event, and each row its live Claim's, worked
// out as the service does. Each Claim's Transcript keeps the last
// TRANSCRIPT_LIMIT events with a rising seq, and is a tail once it has
// dropped some; subscribeTranscript follows it, and dropTranscriptStreams
// stands in for a dropped connection, which reconnects by itself;
// followingTranscripts counts the followers not yet stopped.
//
// Map: the fake's types and limits, then FakeApi in parts marked
// "// ---- <part> ----": seeding, for actions' fake behaviour, the live
// feed, the Api's reads and writes, and internals; then the
// acknowledgements a write returns. grep -n '^ *// ---- ' lists the
// marks, to sed -n the part between two.
import {
  numberOf,
  type Api,
  type EventHandlers,
  type TicketAction,
  type TicketActions,
  type TranscriptHandlers,
} from "./api";
import { fakeActions } from "./fakeActions";
import {
  ApiError,
  DEFAULT_OPTIONS,
  FINISHED,
  FOCUS_CLEARS,
  type ChangeEvent,
  type FindSpecsFilter,
  type FindTicketsFilter,
  type HeldSessionEvent,
  type LatestEvent,
  type LinkedRow,
  type Landing,
  type Mode,
  type NeedsMeReason,
  type NeedsMeRow,
  type NeedsMeView,
  type Options,
  type ProjectRow,
  type ProjectView,
  type RequirementView,
  type SessionEvent,
  type SpecDetail,
  type SpecQuestions,
  type SpecRow,
  SPEC_FIELDS,
  type SpecAcknowledgement,
  type SpecView,
  type StageName,
  type State,
  type StateSummary,
  type SubmitRequirementResult,
  TICKET_FIELDS,
  type TicketAcknowledgement,
  type TicketRow,
  type TicketView,
  type Transcript,
  USAGE_DAYS,
  type UsageFilter,
  type UsageOperation,
  type UsageView,
} from "./types";

// A FakeUsageCall is one agent MCP call as the service records it.
interface FakeUsageCall {
  project: string;
  time: string;
  op: string;
  request_bytes: number;
  response_bytes: number;
  // error is a rejected call's error code.
  error?: string;
}

// localDay numbers t's day in the local time zone, as the service dates Usage.
function localDay(t: Date): number {
  return Math.round(Date.UTC(t.getFullYear(), t.getMonth(), t.getDate()) / 86_400_000);
}

// average is total / n rounded to the nearest whole number, halves up, as the service rounds.
function average(total: number, n: number): number {
  return Math.floor((2 * total + n) / (2 * n));
}

// Every action's fake behaviour registers itself from ./actions/.
import.meta.glob("./actions/*.ts", { eager: true });

interface FakeProject {
  key: string;
  display_name: string;
  created: string;
  mode: Mode;
  archived: boolean;
  archived_at?: string;
  landing: Landing;
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

// LATEST_EVENT_LIMIT is the most characters the latest event's text keeps,
// as core's LatestEventLimit.
const LATEST_EVENT_LIMIT = 120;

// TRANSCRIPT_LIMIT is how many Session Events a Transcript holds, as
// core's TranscriptLimit.
const TRANSCRIPT_LIMIT = 2000;

// transcriptKey keys a Claim's Session Events, as core's markKey does.
const transcriptKey = (ticket: string, claim: number) => `${ticket}#${claim}`;

interface FakeTranscript {
  events: HeldSessionEvent[];
  lastSeq: number;
  tail: boolean;
}

// A FakeFollower is one subscribeTranscript: up while its stream is
// connected, with the last seq it was sent.
interface FakeFollower {
  project: string;
  id: string;
  claim: number;
  h: TranscriptHandlers;
  up: boolean;
  opened: boolean;
  after: number;
}

// READS are the Api's reads, by name, which actions leaves out.
const READS: ReadonlySet<string> = new Set<keyof Api>([
  "listProjects", "needsMe", "findTickets", "getTicket", "getSpec", "findSpecs", "specQuestions", "options", "transcript", "usage",
]);

export class FakeApi implements Api {
  now = new Date("2026-11-07T09:00:00Z");
  // calls records each read and action by name, for tests that check a refetch.
  calls: string[] = [];

  // actions is calls without the reads: the writes made, in order.
  actions(): string[] {
    return this.calls.filter((c) => !READS.has(c));
  }
  private projects = new Map<string, FakeProject>();
  private tickets = new Map<string, TicketView>();
  private specs = new Map<string, SpecView>();
  private groups: { reason: NeedsMeReason; tickets: NeedsMeRow[] }[] = [];
  private subs = new Set<EventHandlers>();
  // transcripts is each Claim's Transcript, keyed by Ticket ID and Claim number.
  private transcripts = new Map<string, FakeTranscript>();
  private followers = new Set<FakeFollower>();
  private opts: Options = { ...DEFAULT_OPTIONS };
  private usageCalls: FakeUsageCall[] = [];

  // ---- seeding ----

  addProject(key: string, display_name = key, opts: { archived?: boolean } = {}) {
    this.projects.set(key, {
      key,
      display_name,
      created: this.now.toISOString(),
      mode: "committed",
      archived: opts.archived ?? false,
      ...(opts.archived ? { archived_at: this.now.toISOString() } : {}),
      landing: { method: "merge", push: false, remote: "origin" },
      next: 1,
    });
    this.emit({ type: "project.changed", project: key });
  }

  // recordCall records an agent's MCP call to project, made now unless
  // the call says when.
  recordCall(project: string, call: Omit<FakeUsageCall, "project" | "time"> & { time?: string }) {
    if (!this.projects.has(project)) throw new Error(`FakeApi: no Project ${project}`);
    this.usageCalls.push({ ...call, project, time: call.time ?? this.now.toISOString() });
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
        const spec = t && !requirement ? this.needsMeSpecOf(t) : undefined;
        return {
          project: this.projectOf(r.id),
          title: t?.title ?? r.id,
          state: t?.state ?? "In Progress",
          since: this.now.toISOString(),
          ...(requirement && { requirement }),
          ...(spec && { spec }),
          ...(t?.labels.includes("prototype") && { prototype: true }),
          ...r,
          reason: g.reason,
        };
      }),
    }));
  }

  // postSessionEvents adds a session's Session Events to the newest Claim
  // with that session ID, live or ended, as a Factory's post would. It
  // counts as activity on the session's live Claims and sends
  // ticket.changed (unthrottled: the fake holds data, not timing). With
  // no such Claim the events are dropped.
  postSessionEvents(sessionId: string, events: SessionEvent[]) {
    if (!sessionId) return;
    let newest: { t: TicketView; number: number; started: string } | undefined;
    for (const t of this.tickets.values()) {
      for (const c of t.claims) {
        if (c.session_id !== sessionId) continue;
        if (!c.ended) c.last_activity = this.now.toISOString();
        if (!newest || c.started > newest.started) newest = { t, number: c.number, started: c.started };
      }
    }
    if (!newest || !events.length) return;
    const k = transcriptKey(newest.t.id, newest.number);
    const tr = this.transcripts.get(k) ?? { events: [], lastSeq: 0, tail: false };
    for (const e of structuredClone(events)) tr.events.push({ seq: ++tr.lastSeq, ...e });
    if (tr.events.length > TRANSCRIPT_LIMIT) {
      tr.events = tr.events.slice(-TRANSCRIPT_LIMIT);
      tr.tail = true;
    }
    this.transcripts.set(k, tr);
    for (const f of this.followers) this.deliver(f);
    this.emit({ type: "ticket.changed", project: this.projectOf(newest.t.id), id: newest.t.id });
  }

  // dropTranscriptStreams drops every connected Transcript stream, as a
  // flaky connection would. Each reconnects by itself, resuming after the
  // last event it was sent.
  dropTranscriptStreams() {
    for (const f of this.followers) {
      if (!f.up) continue;
      f.up = false;
      queueMicrotask(() => this.connect(f));
    }
  }

  // followingTranscripts is how many subscribeTranscript calls are still
  // following, for tests that check a view stops following when it closes.
  followingTranscripts(): number {
    return this.followers.size;
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

  // subscribeTranscript follows a Claim's Transcript as the service's
  // stream does. Like the real one, a stream for an unknown Claim never
  // opens.
  subscribeTranscript(project: string, id: string, claim: number, h: TranscriptHandlers): () => void {
    const f: FakeFollower = { project, id, claim, h, up: false, opened: false, after: 0 };
    this.followers.add(f);
    queueMicrotask(() => this.connect(f));
    return () => this.followers.delete(f);
  }

  private connect(f: FakeFollower) {
    if (!this.followers.has(f)) return;
    try {
      this.claimKey(f.project, f.id, f.claim);
    } catch {
      return;
    }
    f.up = true;
    f.h.onOpen?.({ reconnect: f.opened });
    f.opened = true;
    this.deliver(f);
  }

  // deliver sends a connected follower the events after the last it was
  // sent, after a tail notice when some between were dropped first.
  private deliver(f: FakeFollower) {
    if (!f.up) return;
    const tr = this.transcripts.get(this.claimKey(f.project, f.id, f.claim));
    if (!tr) return;
    if (tr.events.length && tr.events[0].seq > f.after + 1) f.h.onTail?.();
    for (const e of tr.events) {
      if (e.seq <= f.after) continue;
      f.after = e.seq;
      f.h.onEvent(structuredClone(e));
    }
  }

  // ---- the Api ----

  async transcript(project: string, id: string, claim: number): Promise<Transcript> {
    this.calls.push("transcript");
    const tr = this.transcripts.get(this.claimKey(project, id, claim));
    return structuredClone({ events: tr?.events ?? [], tail: tr?.tail ?? false });
  }

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
    return { spec: acknowledgeSpecCreate(spec), stages: stages.map(acknowledgeTicketCreate) };
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
    const before = structuredClone(t);
    return acknowledge(before, handler(this, t, body));
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

  async setLandingMethod(key: string, landing: Landing): Promise<ProjectView> {
    this.calls.push("setLandingMethod");
    if (landing.method !== "merge" && landing.method !== "branch") {
      throw new ApiError(422, "invalid", `Landing Method "${landing.method}" is neither "merge" nor "branch"`, "method");
    }
    const l = { method: landing.method, push: landing.push, remote: landing.remote.trim() };
    if (!l.remote) {
      if (l.push) throw new ApiError(422, "invalid", 'Push needs a git remote to push to, such as "origin"', "remote");
      l.remote = "origin";
    }
    const p = this.writable(key);
    if (JSON.stringify(p.landing) !== JSON.stringify(l)) {
      p.landing = l;
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

  async options(): Promise<Options> {
    this.calls.push("options");
    return { ...this.opts };
  }

  async setOptions(change: Partial<Options>): Promise<Options> {
    this.calls.push("setOptions");
    for (const name of Object.keys(change)) {
      if (!(name in DEFAULT_OPTIONS)) {
        throw new ApiError(422, "invalid", `unknown field "${name}"; "${name}" is not an Option (the Options are: ${Object.keys(DEFAULT_OPTIONS).join(", ")})`, name);
      }
    }
    const { focus_clear } = change;
    if (focus_clear !== undefined && !FOCUS_CLEARS.includes(focus_clear)) {
      throw new ApiError(422, "invalid", `focus_clear "${focus_clear}" is not an Option's value; allowed: ${FOCUS_CLEARS.join(", ")}`, "focus_clear");
    }
    const next = { ...this.opts, ...change };
    if (Object.keys(DEFAULT_OPTIONS).some((k) => next[k as keyof Options] !== this.opts[k as keyof Options])) {
      this.opts = next;
      this.emit({ type: "options.changed" });
    }
    return { ...this.opts };
  }

  async usage(filter: UsageFilter = {}): Promise<UsageView> {
    this.calls.push("usage");
    const days = filter.days ?? USAGE_DAYS[0];
    if (!USAGE_DAYS.includes(days)) throw new ApiError(422, "invalid", `days must be 7, 30 or 90, not ${days}`, "days");
    if (filter.project && !this.projects.has(filter.project)) throw new ApiError(404, "not_found", `no Project "${filter.project}"`);
    const today = localDay(this.now);
    const rows = new Map<string, UsageOperation>();
    for (const c of this.usageCalls) {
      const ago = today - localDay(new Date(c.time));
      if ((filter.project && c.project !== filter.project) || ago < 0 || ago >= days) continue;
      const row = rows.get(c.op) ?? { op: c.op, calls: 0, request_bytes: 0, average_request_bytes: 0, response_bytes: 0, average_response_bytes: 0, errors: 0 };
      row.calls++;
      row.request_bytes += c.request_bytes;
      row.response_bytes += c.response_bytes;
      if (c.error) row.errors++;
      rows.set(c.op, row);
    }
    const operations = [...rows.values()]
      .map((r) => ({ ...r, average_request_bytes: average(r.request_bytes, r.calls), average_response_bytes: average(r.response_bytes, r.calls) }))
      .sort((a, b) => b.calls - a.calls || (a.op < b.op ? -1 : 1));
    return { operations, kinds: [], sessions: [], days: [] };
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
      ...this.liveLatest(t),
    }));
  }

  // liveLatest is a row's latest_event: its live Claim's, if it has one.
  private liveLatest(t: TicketView): { latest_event?: LatestEvent } {
    const live = t.claims.find((c) => !c.ended);
    const latest_event = live && this.latestEvent(t.id, live.number);
    return latest_event ? { latest_event } : {};
  }

  // latestEvent is a Claim's latest Session Event shown short, as core's
  // latestEvent: a tool result as "<Tool> result" or "<Tool> failed",
  // naming the tool from the call it answers; anything else as its first
  // non-empty line; cut to LATEST_EVENT_LIMIT characters.
  private latestEvent(ticket: string, claim: number): LatestEvent | undefined {
    const events = this.transcripts.get(transcriptKey(ticket, claim))?.events ?? [];
    const e = events.at(-1);
    if (!e) return undefined;
    let line: string;
    if (e.kind === "tool_result") {
      const call = [...events].reverse().find((c) => c.kind === "tool_call" && c.tool && c.tool_use_id === e.tool_use_id);
      const tool = e.tool || (e.tool_use_id && call?.tool) || "Tool";
      line = `${tool} ${e.is_error ? "failed" : "result"}`;
    } else {
      line = e.text.split("\n").map((l) => l.trim()).find((l) => l) ?? "";
    }
    const chars = [...line];
    const text = chars.length <= LATEST_EVENT_LIMIT ? line : chars.slice(0, LATEST_EVENT_LIMIT - 1).join("") + "…";
    return { kind: e.kind, text, time: e.time };
  }

  // requirementOf is a Needs Me row's requirement field: set when the
  // Ticket's Spec is a Requirement, with stage only on a Stage Ticket.
  private requirementOf(t: TicketView): NeedsMeRow["requirement"] {
    if (!this.ticketsOf(t.spec.id).some((s) => stageOf(s))) return undefined;
    const stage = stageOf(t);
    return { spec: t.spec.id, title: this.specs.get(t.spec.id)?.title ?? t.spec.title, ...(stage && { stage }) };
  }

  // needsMeSpecOf is a Needs Me row's spec field, for a Ticket whose Spec
  // isn't a Requirement: set when the Ticket is on a Map, or is a Map
  // Ticket or a Prototype.
  private needsMeSpecOf(t: TicketView): NeedsMeRow["spec"] {
    const s = this.specs.get(t.spec.id);
    const map = !!s?.labels.includes("wayfinder:map");
    if (!map && !t.labels.some((l) => l === "prototype" || /^wayfinder:(chart|grilling|research|task)$/.test(l))) return undefined;
    return { spec: t.spec.id, title: s?.title ?? t.spec.title, ...(map && { map }) };
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

  // claimKey is a Claim's Transcript key; an unknown Project, Ticket or
  // Claim is not_found, as on the service's Transcript reads.
  private claimKey(project: string, id: string, claim: number) {
    const t = this.projects.has(project) ? this.tickets.get(`${project}-${numberOf(id)}`) : undefined;
    if (!t?.claims.some((c) => c.number === claim)) {
      throw new ApiError(404, "not_found", `no Claim ${claim} of ${project}-${numberOf(id)}`);
    }
    return transcriptKey(t.id, claim);
  }

  private view(t: TicketView): TicketView {
    const reason = this.rows().find((r) => r.id === t.id)?.reason;
    const v = structuredClone(t);
    for (const c of v.claims) {
      const latest = this.latestEvent(t.id, c.number);
      if (latest) c.latest_event = latest;
    }
    if (reason) v.needs_me_reason = reason;
    else delete v.needs_me_reason;
    return v;
  }
}

// ---- acknowledgements ----

// written is a write's acknowledgement, as the service derives it: the
// fields, in the view's order, whose value differs between the views
// before and after the write, except those unlisted.
function written<V extends SpecView | TicketView>(fields: readonly (keyof V & string)[], before: V, after: V, unlisted: string[]) {
  const changed = fields.filter((f) => !unlisted.includes(f) && JSON.stringify(before[f]) !== JSON.stringify(after[f]));
  const edit = changed.includes("edits") ? after.edits.at(-1)?.number : undefined;
  return { id: after.id, updated: after.updated, changed, ...(edit && { edit }) };
}

// acknowledge is a Ticket write's result: written, never updated, with
// the Ticket's State.
function acknowledge(before: TicketView, after: TicketView): TicketAcknowledgement {
  return { ...written(TICKET_FIELDS, before, after, ["updated"]), state: after.state };
}

// A create's acknowledgement is written from the empty record's view, as
// the service's is, never listing the id and times every create sets.
const CREATED = ["id", "created", "updated"];
const EMPTY_SPEC: SpecView = { id: "", title: "", labels: [], tags: {}, created: "", updated: "", body: "", edits: [] };
const EMPTY_TICKET: TicketView = {
  id: "", title: "", state: "" as State, spec: { id: "", title: "" }, labels: [], tags: {}, blocked_by: [], blockers: [],
  replacements: [], created: "", updated: "", description: "", claims: [], rounds: [], failure_reports: [], comments: [],
  moves: [], edits: [], blocked: false, claimable: false,
};

function acknowledgeSpecCreate(after: SpecView): SpecAcknowledgement {
  return written(SPEC_FIELDS, EMPTY_SPEC, after, CREATED);
}

function acknowledgeTicketCreate(after: TicketView): TicketAcknowledgement {
  return { ...written(TICKET_FIELDS, EMPTY_TICKET, after, CREATED), state: after.state };
}
