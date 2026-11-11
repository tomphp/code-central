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
// submitRequirement writes a Spec and its three Stage Tickets (only the
// spec and tickets Stages, and a map Tag, when written from a Map), and
// submitMap a Map's Spec and its Charting Ticket; a Map's read derives its
// Map block, and its Tickets' rows their node type. submitPrototype
// writes a Ready prototype Ticket on any Spec.
//
// recordCall stands in for an agent's MCP call, which the service records
// as Usage itself, and postUsage for a Factory's POST /api/usage, filed
// under the newest Claim with its session ID; usage reads them back as the
// service does: over the last days local days, for one Project or all of
// them.
//
// The Options start at their defaults; setOptions validates as the
// service does, and sends options.changed only when something changed.
// setCITracking runs the CI check when it turns CI Tracking on, or
// changes what it watches while on, as the service does: it passes
// unless seedCICheck has given it a refusal. setGitHubIssues runs the
// Issues check likewise, refused by seedIssuesCheck, and creates the
// Issues Spec the first time it turns GitHub Issues on. setModels
// validates as the service does, effort against the Project's reported
// levels, EFFORT_LEVELS until reportModels has given some, and setTags
// checks a Ticket's effort Tag the same way. getTicket, the UI's read,
// carries the Ticket's Model Choice, worked out as the service does.
// addIncomingIssue plays the Integration's create_linked_ticket, marking
// a New issue as the service does, and keep_as_draft, to_requirement and
// wont_fix clear it; to_requirement writes a Requirement whose Spec has
// an outbound Link.
//
// reportPushedCommit and reportCIRuns work a Ticket's CI result out as
// the service does, with its failure options while it is CI failed;
// land_anyway records the human's choice on the latest pushed commit;
// rerun_ci records a Re-run request for its failed runs, which reads as
// running until a later attempt of each is reported.
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
  type ChoiceValue,
  DEFAULT_OPTIONS,
  TRIAGE_ACTIONS,
  FINISHED,
  FOCUS_CLEARS,
  GUIDED_ON,
  GUIDED_TAG,
  type ChangeEvent,
  type FindSpecsFilter,
  type FindTicketsFilter,
  type HeldSessionEvent,
  type LatestEvent,
  type LinkedRow,
  type LinkView,
  type TriageAction,
  type CIConclusion,
  type CIFailureOption,
  type CIResult,
  type CIProvider,
  type CIRule,
  type CIRunView,
  type CIRunRow,
  type CIOverviewFilter,
  type CIOverview,
  CI_VIEW_DAYS,
  type CIRuleChange,
  type CISettings,
  type GitHubIssuesChange,
  type GitHubIssuesSettings,
  type CIWatcherReport,
  type CIWatcherStatus,
  type IssueWatcherReport,
  type IssueWatcherStatus,
  type PushedCommitView,
  type CITrackingChange,
  type Landing,
  MAP_LABEL,
  type MapView,
  type Mode,
  type NeedsMeReason,
  type NeedsMeRow,
  type NeedsMeView,
  type NodeType,
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
  type SubmitMapResult,
  type SubmitRequirementResult,
  type TargetView,
  TICKET_FIELDS,
  type TicketAcknowledgement,
  type TicketRow,
  type TicketView,
  type Transcript,
  USAGE_DAYS,
  USAGE_KEPT_DAYS,
  EFFORT_LEVELS,
  sessionKey,
  type ModelRow,
  type Models,
  type FactoryStatus,
  type HeartbeatReport,
  type ModelsReport,
  type ReportedModels,
  type UsageDay,
  type UsageDayTokens,
  type UsageFilter,
  type UsageKind,
  type UsageOperation,
  type UsageResponse,
  type UsageSession,
  type SessionUsageFilter,
  type SessionUsageView,
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

// A FakeUsageRecord is a posted response as the service records it, under its Claim.
interface FakeUsageRecord extends UsageResponse {
  project: string;
  ticket: string;
  claim: number;
  session_id: string;
}

// KIND_ORDER is the order Usage lists kinds of session in; any other kind follows, by name.
const KIND_ORDER = ["grilling", "spec", "tickets", "implementation", "review", "subagent"];

// RECENT_SESSIONS is how many sessions a Usage read lists.
const RECENT_SESSIONS = 50;

// localDay numbers t's day in the local time zone, as the service dates Usage.
function localDay(t: Date): number {
  return Math.round(Date.UTC(t.getFullYear(), t.getMonth(), t.getDate()) / 86_400_000);
}

// average is total / n rounded to the nearest whole number, halves up, as the service rounds.
function average(total: number, n: number): number {
  return Math.floor((2 * total + n) / (2 * n));
}

// contextSize is how many tokens a response read: input, cache write and cache read.
function contextSize(r: UsageResponse): number {
  return r.input_tokens + r.cache_write_tokens + r.cache_read_tokens;
}

// usageSessions are the sessions the responses belong to, newest first, as
// the service works them out: a session is its session ID plus its agent,
// with the Claim and kind of its latest response, and its models in
// order of first use, whatever order the responses were posted in.
function usageSessions(responses: FakeUsageRecord[]): UsageSession[] {
  const byKey = new Map<string, UsageSession>();
  for (const r of [...responses].sort((a, b) => Date.parse(a.time) - Date.parse(b.time))) {
    const key = `${r.session_id}\u0000${r.agent ?? ""}`;
    let s = byKey.get(key);
    if (!s) {
      s = { project: "", ticket: "", claim: 0, session_id: r.session_id, ...(r.agent ? { agent: r.agent } : {}), kind: "", started: r.time, peak: 0, responses: 0, models: [] };
      byKey.set(key, s);
    }
    s.responses++;
    s.peak = Math.max(s.peak, contextSize(r));
    if (r.model && !s.models.includes(r.model)) s.models.push(r.model);
    Object.assign(s, { project: r.project, ticket: r.ticket, claim: r.claim, kind: r.kind });
  }
  const by = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0);
  return [...byKey.values()]
    .sort((a, b) => Date.parse(b.started) - Date.parse(a.started) || by(a.session_id, b.session_id) || by(a.agent ?? "", b.agent ?? ""));
}

// usageKinds is each kind of session's Context Size figures, in KIND_ORDER.
function usageKinds(sessions: UsageSession[]): UsageKind[] {
  const byKind = new Map<string, { k: UsageKind; total: number }>();
  for (const s of sessions) {
    const e = byKind.get(s.kind) ?? { k: { kind: s.kind, sessions: 0, average_peak: 0, largest_peak: 0 }, total: 0 };
    e.k.sessions++;
    e.k.largest_peak = Math.max(e.k.largest_peak, s.peak);
    e.total += s.peak;
    byKind.set(s.kind, e);
  }
  return [...byKind.values()]
    .map((e) => ({ ...e.k, average_peak: average(e.total, e.k.sessions) }))
    .sort((a, b) => compareKinds(a.kind, b.kind));
}

// compareKinds orders kinds by KIND_ORDER, then any other by name.
function compareKinds(a: string, b: string): number {
  const rank = (kind: string) => (KIND_ORDER.includes(kind) ? KIND_ORDER.indexOf(kind) : KIND_ORDER.length);
  return rank(a) - rank(b) || (a < b ? -1 : a > b ? 1 : 0);
}

// dateOf is t's local day as YYYY-MM-DD, as the service names a day.
function dateOf(t: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${t.getFullYear()}-${pad(t.getMonth() + 1)}-${pad(t.getDate())}`;
}

// usageDays is a row for each of the last days local days to today,
// oldest first, with the responses' tokens that day summed by kind.
function usageDays(today: Date, days: number, responses: FakeUsageRecord[]): UsageDay[] {
  const byDay = new Map<string, Map<string, UsageDayTokens>>();
  for (const r of responses) {
    const day = dateOf(new Date(r.time));
    const kinds = byDay.get(day) ?? new Map<string, UsageDayTokens>();
    const k = kinds.get(r.kind) ?? { kind: r.kind, input_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 0 };
    k.input_tokens += r.input_tokens;
    k.cache_write_tokens += r.cache_write_tokens;
    k.cache_read_tokens += r.cache_read_tokens;
    k.output_tokens += r.output_tokens;
    kinds.set(r.kind, k);
    byDay.set(day, kinds);
  }
  const rows: UsageDay[] = [];
  for (let i = days - 1; i >= 0; i--) {
    const day = dateOf(new Date(today.getFullYear(), today.getMonth(), today.getDate() - i));
    rows.push({ day, kinds: [...(byDay.get(day)?.values() ?? [])].sort((a, b) => compareKinds(a.kind, b.kind)) });
  }
  return rows;
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
  ci: CISettings;
  github_issues: GitHubIssuesSettings;
  models: Models;
  reported_models: ReportedModels;
  factory_status: FactoryStatus;
  next: number;
}

// FAILED_CONCLUSIONS are the conclusions that fail a CI result, as
// store's CIConclusion.Failed; a cancelled run makes it cancelled
// instead. RERUN_CONCLUSIONS are those a Re-run request covers.
const FAILED_CONCLUSIONS: CIConclusion[] = ["failure", "timed_out"];
const RERUN_CONCLUSIONS: CIConclusion[] = [...FAILED_CONCLUSIONS, "cancelled"];

export type SeedTicket = Partial<TicketView> & { title: string };
export type SeedSpec = Partial<SpecView> & { title: string };

// The Stages in order, with the title each Stage Ticket's starts with.
const STAGES: { name: StageName; title: string }[] = [
  { name: "grilling", title: "Grilling" },
  { name: "spec", title: "Spec" },
  { name: "tickets", title: "Tickets" },
];
const stageOf = (t: { labels: string[] }) => STAGES.find((s) => t.labels.includes(`stage:${s.name}`))?.name;
// The Map's Ticket types and its body's sections, as core's WayfinderTypes
// and MapSections.
const WAYFINDER_TYPES = ["chart", "grilling", "research", "task"] as const;
// KINDS_OF_WORK are the Stage Guide names, in the service's order: the
// Models rows a Project has.
const KINDS_OF_WORK: string[] = ["grilling", "spec", "tickets", ...WAYFINDER_TYPES.map((t) => `map-${t}`), "prototype", "implementation"];
// kindOf is a Ticket's kind of work, its Stage Guide name, from its first
// reserved type Label, as core's GuideName.
const kindOf = (labels: string[]): string => {
  for (const l of labels) {
    if (l === "prototype") return l;
    const [prefix, name] = [l.slice(0, l.indexOf(":") + 1), l.slice(l.indexOf(":") + 1)];
    if (prefix === "wayfinder:" && (WAYFINDER_TYPES as readonly string[]).includes(name)) return `map-${name}`;
    if (prefix === "stage:" && STAGES.some((s) => s.name === name)) return name;
  }
  return "implementation";
};
// choose is the first of a Model Choice value's tiers set, with its
// source, as core's choose: the Tag, the row for the kind of work, then
// the default row.
const choose = (tag: string | undefined, kindName: string, kindValue: string, def: string): ChoiceValue =>
  tag
    ? { value: tag, source: "ticket_tag" }
    : kindValue
      ? { value: kindValue, source: "project_kind", kind: kindName }
      : def
        ? { value: def, source: "project_default" }
        : { source: "none" };
const MAP_SECTIONS = ["Destination", "Notes", "Decisions so far", "Not yet specified", "Out of scope"];
// specTitle is the text's first non-blank line without Markdown heading
// marks, as core's specTitle.
const specTitle = (text: string) =>
  text
    .split("\n")
    .map((l) => l.trim().replace(/^#+/, "").trim())
    .find((l) => l) ?? "";
const nodeTypeOf = (t: { labels: string[] }): NodeType =>
  t.labels.includes("prototype") ? "prototype" : (WAYFINDER_TYPES.find((n) => t.labels.includes(`wayfinder:${n}`)) ?? "ticket");
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
  "listProjects", "needsMe", "findTickets", "getTicket", "getSpec", "findSpecs", "specQuestions", "options", "transcript", "usage", "sessionUsage", "ciOverview",
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
  // pushedCommits are each Ticket's pushed commits, oldest first, and
  // ciRuns each Project's CI Runs (code-central-256).
  private pushedCommits = new Map<string, PushedCommitView[]>();
  private ciRuns = new Map<string, CIRunView[]>();
  // landedAnyway is each "<ticket>@<commit>" the human chose Land anyway
  // for (code-central-261).
  private landedAnyway = new Set<string>();
  // reruns are each Ticket's Re-run request (code-central-265): its
  // pushed commit and the failed run attempts it asked for.
  private reruns = new Map<string, { commit: string; runs: { id: number; attempt: number }[] }>();
  private specs = new Map<string, SpecView>();
  private groups: { reason: NeedsMeReason; tickets: NeedsMeRow[] }[] = [];
  private subs = new Set<EventHandlers>();
  // transcripts is each Claim's Transcript, keyed by Ticket ID and Claim number.
  private transcripts = new Map<string, FakeTranscript>();
  private followers = new Set<FakeFollower>();
  private opts: Options = { ...DEFAULT_OPTIONS };
  // ciCheck is the refusal the CI check answers with; none passes.
  private ciCheck?: { reason: string; fix: string; field?: "tracking" | "repository" | "project_url" };
  // issuesCheck is the refusal the Issues check answers with; none passes.
  private issuesCheck?: { reason: string; fix: string; field?: "on" | "repository" };
  private usageCalls: FakeUsageCall[] = [];
  private usageResponses: FakeUsageRecord[] = [];

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
      ci: {
        tracking: false,
        provider: "github_actions",
        repository: "",
        project_url: "",
        pat_env: "ADO_PAT",
        ignored_workflows: [],
        unblock_on_push: false,
        push_remote_url: "",
        rules: [],
        watcher: { state: "not_reported" },
      },
      github_issues: { on: false, repository: "", label: "", new_issues_need_me: false, watcher: { state: "not_reported" } },
      models: { default: { model: "", effort: "" }, kinds: KINDS_OF_WORK.map((kind) => ({ kind, model: "", effort: "" })) },
      reported_models: { effort_levels: [...EFFORT_LEVELS], aliases: [], model_ids: [], model_flag: "", effort_flag: "" },
      factory_status: { status: "offline", factories: [] },
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

  // seedCICheck sets how the CI Watcher's check answers when CI Tracking
  // is turned on: refused with this reason and fix, on field (tracking
  // unless given; the service gives repository when gh can't read it),
  // or, with none, passed.
  seedCICheck(refused?: { reason: string; fix: string; field?: "tracking" | "repository" | "project_url" }) {
    this.ciCheck = refused;
  }

  // seedIssuesCheck sets how the Issue Watcher's check answers when GitHub
  // Issues is turned on: refused with this reason and fix, on field (on
  // unless given; the service gives repository when the problem is the
  // repository's), or, with none, passed.
  seedIssuesCheck(refused?: { reason: string; fix: string; field?: "on" | "repository" }) {
    this.issuesCheck = refused;
  }

  // seedModelIDs adds model IDs to key's Reported Models, as the
  // service collects them from usage.
  seedModelIDs(key: string, ids: string[]) {
    const r = this.project(key).reported_models;
    r.model_ids = [...new Set([...r.model_ids, ...ids])];
    this.emit({ type: "project.changed", project: key });
  }

  // seedCIWatcherCrash shows key's CI Watcher as the service reports one
  // that crashed (code-central-255), restarted restarts times.
  seedCIWatcherCrash(key: string, restarts: number, lastLine: string) {
    const p = this.project(key);
    const times = restarts === 1 ? "time" : "times";
    this.setCI(p, {
      ...p.ci,
      watcher: {
        state: "error",
        problem: "crashed",
        message: `The CI Watcher crashed, restarted ${restarts} ${times}; the last line it wrote: ${lastLine}`,
        fix: "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log.",
        since: this.now.toISOString(),
      },
    });
  }

  // seedIssueWatcherCrash shows key's Issue Watcher as the service
  // reports one that crashed (code-central-285), restarted restarts times.
  seedIssueWatcherCrash(key: string, restarts: number, lastLine: string) {
    const p = this.project(key);
    const times = restarts === 1 ? "time" : "times";
    p.github_issues = {
      ...p.github_issues,
      watcher: {
        state: "error",
        problem: "crashed",
        message: `The Issue Watcher crashed, restarted ${restarts} ${times}; the last line it wrote: ${lastLine}`,
        fix: "Code Central restarts it by itself, waiting longer after each crash. Its output is in the service's log.",
        since: this.now.toISOString(),
      },
    };
    this.emit({ type: "project.changed", project: key });
  }

  // seedOptions sets the Options as a store would hold them, recording no
  // action and sending no notice.
  seedOptions(options: Partial<Options>) {
    this.opts = { ...this.opts, ...options };
  }

  // postSessionEvents adds a session's Session Events to the newest Claim
  // with that session ID, live or ended, as a Factory's post would. It
  // counts as activity on the session's live Claims and sends
  // ticket.changed (unthrottled: the fake holds data, not timing). With
  // no such Claim the events are dropped.
  postSessionEvents(sessionId: string, events: SessionEvent[]) {
    const newest = this.pingSession(sessionId);
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

  // postUsage records a session's model responses under the newest Claim
  // with that session ID, live or ended, as a Factory's POST /api/usage
  // would. It counts as activity on the session's live Claims. With no
  // such Claim the responses are dropped.
  postUsage(sessionId: string, responses: UsageResponse[]) {
    const newest = this.pingSession(sessionId);
    if (!newest) return;
    const claim = { project: this.projectOf(newest.t.id), ticket: newest.t.id, claim: newest.number, session_id: sessionId };
    for (const r of structuredClone(responses)) this.usageResponses.push({ ...r, ...claim });
  }

  // pingSession marks activity on a session's live Claims, as a ping
  // does, and returns its newest Claim, live or ended.
  private pingSession(sessionId: string): { t: TicketView; number: number; started: string } | undefined {
    if (!sessionId) return undefined;
    let newest: { t: TicketView; number: number; started: string } | undefined;
    for (const t of this.tickets.values()) {
      for (const c of t.claims) {
        if (c.session_id !== sessionId) continue;
        if (!c.ended) c.last_activity = this.now.toISOString();
        if (!newest || c.started > newest.started) newest = { t, number: c.number, started: c.started };
      }
    }
    return newest;
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

  // landAnyway records the human's Land anyway on t's latest pushed
  // commit, refused unless t is offered it, as core's LandAnyway is, and
  // takes t out of Needs Me.
  landAnyway(t: TicketView): TicketView {
    const key = this.projectOf(t.id);
    const latest = this.pushedCommits.get(t.id)?.at(-1);
    if (t.land_anyway && t.state === "Accepted") return this.view(t);
    if (!latest || !t.ci_failure_options?.includes("land_anyway")) {
      throw new ApiError(409, "action_not_allowed", `${t.id} isn't offered Land anyway: it needs a failed CI result, Accepted under Merge`);
    }
    this.landedAnyway.add(`${t.id}@${latest.commit}`);
    this.dropNeedsMe(t.id);
    return this.trackCI(key, t);
  }

  // addIncomingIssue is the Integration's create_linked_ticket on spec,
  // in Draft: on the Issues Spec it is an Incoming Issue, offered the
  // triage actions, and with New issues need me on it awaits triage and
  // joins Needs Me as a New issue, just before any closed externally
  // group, as core's CreateLinkedTicket and Needs Me do.
  addIncomingIssue(project: string, spec: string, title: string, link: { integration: string; id: string; url: string }): TicketView {
    const issues = this.project(project).github_issues;
    const incoming = spec === issues.spec;
    const awaiting = incoming && issues.new_issues_need_me;
    const t = this.addTicket(project, {
      title,
      spec: { id: spec, title: this.specs.get(spec)?.title ?? "" },
      link: { ...link, external_state: "open", detached: false, outbound: false },
      ...(incoming && { triage_actions: [...TRIAGE_ACTIONS] }),
      ...(awaiting && { awaiting_triage: true }),
    });
    if (awaiting) {
      const groups: { reason: NeedsMeReason; tickets: SeedRow[] }[] = this.needsMeGroups;
      const row = { id: t.id, link: t.link, since: t.created };
      const group = groups.find((g) => g.reason === "new_issue");
      const closed = groups.findIndex((g) => g.reason === "closed_externally");
      if (group) group.tickets.push(row);
      else groups.splice(closed < 0 ? groups.length : closed, 0, { reason: "new_issue", tickets: [row] });
      this.setNeedsMe(groups);
    }
    return t;
  }

  // editTicket applies the fields that differ, recording an edit by human
  // with their previous values, as core's EditTicket does: an empty title
  // is refused, a Ticket whose Link owns its definition is refused, and a
  // change that changes nothing writes nothing.
  editTicket(t: TicketView, change: { title?: string; description?: string }): TicketView {
    if (change.title !== undefined && !change.title.trim()) throw new ApiError(422, "invalid", "a Ticket needs a title", "title");
    this.definitionOwned(t.id, t.link);
    const previous: Record<string, string> = {};
    const patch: Partial<TicketView> = {};
    if (change.title !== undefined && change.title !== t.title) [previous.title, patch.title] = [t.title, change.title];
    if (change.description !== undefined && change.description !== t.description) {
      [previous.description, patch.description] = [t.description, change.description];
    }
    if (!Object.keys(previous).length) return t;
    const edits = [...t.edits, { number: t.edits.length + 1, actor: "human", time: this.now.toISOString(), previous }];
    return this.updateTicket(t.id, { ...patch, edits });
  }

  // definitionOwned refuses a write to a Spec or Ticket whose Link owns
  // its definition, as the service does for the human.
  private definitionOwned(id: string, link?: LinkView) {
    if (link && !link.detached && !link.outbound) {
      throw new ApiError(
        409,
        "action_not_allowed",
        `${id} is linked to ${link.integration} ${link.id}, which owns its definition: change it there, or unlink it first (this request is made as human)`,
      );
    }
  }

  // keepAsDraft clears an Incoming Issue's awaiting triage and takes it
  // out of Needs Me, refused on anything else, as core's KeepAsDraft is.
  keepAsDraft(t: TicketView): TicketView {
    if (!t.triage_actions?.includes("keep_as_draft")) {
      throw new ApiError(409, "action_not_allowed", `${t.id} is not an Incoming Issue: only a linked Ticket on the Project's Issues Spec is triaged`);
    }
    if (!t.awaiting_triage) return this.view(t);
    this.dropNeedsMe(t.id);
    const { awaiting_triage: _a, ...rest } = this.tickets.get(t.id)!;
    this.tickets.set(t.id, rest);
    return this.updateTicket(t.id, {});
  }

  // wontFix Cancels an Incoming Issue via wont_fix, with reason as its
  // comment, clearing its awaiting triage and taking it out of Needs Me,
  // refused on anything else, as core's WontFix is.
  wontFix(t: TicketView, reason?: string): TicketView {
    return this.triageCancel(t, "wont_fix", reason, {});
  }

  // toRequirement makes an Incoming Issue a Requirement, as core's
  // ToRequirement does: the issue's title as a heading, then its
  // description, then the context under its own heading, on a Spec with
  // an outbound Link to the issue; the Incoming Issue's Link is detached,
  // and it is Cancelled with the grilling Stage as its Replacement.
  toRequirement(t: TicketView, context?: string): TicketView {
    const link = this.incoming(t, "to_requirement");
    const extra = context?.trim();
    const parts = [`# ${t.title}`, t.description.trim(), ...(extra ? ["## Context", extra] : [])].filter(Boolean);
    const { stages } = this.addRequirement(this.projectOf(t.id), { title: t.title, body: parts.join("\n\n") + "\n", link: { ...link, outbound: true } });
    return this.triageCancel(t, "to_requirement", extra, { replacements: [stages[0].id], link: { ...link, detached: true } });
  }

  // incoming is t's Link, refusing the triage action via unless t is an
  // Incoming Issue.
  private incoming(t: TicketView, via: TriageAction): LinkView {
    if (!t.link || !t.triage_actions?.includes(via)) {
      const why = t.link?.detached
        ? `${t.id} was unlinked from ${t.link.integration} ${t.link.id}, so it is no longer an Incoming Issue to triage`
        : `${t.id} is not an Incoming Issue: only a linked Ticket on the Project's Issues Spec is triaged`;
      throw new ApiError(409, "action_not_allowed", why);
    }
    return t.link;
  }

  // triageCancel Cancels an Incoming Issue via the triage action, with
  // comment as its comment and patch applied, clearing its awaiting
  // triage and taking it out of Needs Me, refused on anything else.
  private triageCancel(t: TicketView, via: TriageAction, comment: string | undefined, patch: Partial<TicketView>): TicketView {
    this.incoming(t, via);
    const at = this.now.toISOString();
    const body = comment?.trim();
    this.dropNeedsMe(t.id);
    const { awaiting_triage: _a, triage_actions: _t, ...rest } = this.tickets.get(t.id)!;
    this.tickets.set(t.id, rest);
    return this.updateTicket(t.id, {
      ...patch,
      state: "Cancelled",
      moves: [...t.moves, { from: t.state, to: "Cancelled", actor: "human", time: at, via }],
      comments: body ? [...t.comments, { number: t.comments.length + 1, actor: "human", time: at, body }] : t.comments,
    });
  }

  // rerunCI records a Re-run request for each failed run of t's latest
  // pushed commit, refused unless t is offered it, as core's RerunCI is,
  // and takes t out of Needs Me.
  rerunCI(t: TicketView): TicketView {
    const key = this.projectOf(t.id);
    const latest = this.pushedCommits.get(t.id)?.at(-1);
    if (latest && t.state === "Accepted" && this.pendingReruns(key, t.id, latest.commit).length > 0) return this.view(t);
    if (!latest || !t.ci_failure_options?.includes("rerun_ci")) {
      throw new ApiError(409, "action_not_allowed", `${t.id} has no failed CI result to re-run`);
    }
    const runs = this.countedRuns(key, t.id, latest.commit)
      .filter((r) => r.conclusion !== undefined && RERUN_CONCLUSIONS.includes(r.conclusion))
      .map((r) => ({ id: r.id, attempt: r.attempt }))
      .sort((a, b) => a.id - b.id);
    this.reruns.set(t.id, { commit: latest.commit, runs });
    this.dropNeedsMe(t.id);
    return this.trackCI(key, t);
  }

  // pendingReruns are the Ticket's Re-run requests for commit with no
  // later attempt of their run reported, as core's pendingReruns are.
  private pendingReruns(key: string, id: string, commit: string): { id: number; attempt: number }[] {
    const req = this.reruns.get(id);
    if (!req || req.commit !== commit) return [];
    const mine = (this.ciRuns.get(key) ?? []).filter((r) => r.ticket === id && r.commit === commit);
    return req.runs.filter((a) => !mine.some((r) => r.id === a.id && r.attempt > a.attempt));
  }

  // countedRuns are the runs of commit that count towards the Ticket's CI
  // result: those not ignored, each at its latest attempt.
  private countedRuns(key: string, id: string, commit: string): CIRunView[] {
    const counted = new Map<number, CIRunView>();
    for (const r of this.ciRuns.get(key) ?? []) {
      if (r.ticket !== id || r.commit !== commit || r.ignored) continue;
      if ((counted.get(r.id)?.attempt ?? 0) < r.attempt) counted.set(r.id, r);
    }
    return [...counted.values()];
  }

  // newFixTicket opens a Ready Ticket on t's Spec carrying note, and
  // records on t the same acceptance as Land anyway, with a comment naming
  // the fix Ticket, refused unless t is offered it, as core's NewFixTicket
  // is. It returns the fix Ticket.
  newFixTicket(t: TicketView, note: string | undefined): TicketView {
    const key = this.projectOf(t.id);
    const latest = this.pushedCommits.get(t.id)?.at(-1);
    if (!note?.trim()) {
      throw new ApiError(422, "invalid", "a fix Ticket needs a note saying what failed, such as the failed CI run's link and its failed jobs", "note");
    }
    if (!latest || !t.ci_failure_options?.includes("new_fix_ticket")) {
      throw new ApiError(409, "action_not_allowed", `${t.id} has no failed CI result waiting on you to fix`);
    }
    const fix = this.addTicket(key, {
      title: `Fix the CI failure of ${t.id}: ${t.title}`,
      state: "Ready",
      spec: t.spec,
      claimable: true,
      description: `${t.id}'s CI failed, and it lands anyway: fix the failure forward, on top of its work in the base.\n\n${note}`,
    });
    this.landedAnyway.add(`${t.id}@${latest.commit}`);
    this.dropNeedsMe(t.id);
    const at = this.now.toISOString();
    const body = `New fix Ticket: ${fix.id} fixes this CI failure forward, and ${t.id} lands anyway.`;
    this.updateTicket(t.id, { comments: [...t.comments, { number: t.comments.length + 1, actor: "human", time: at, body }] });
    this.trackCI(key, this.ticket(key, t.id));
    return fix;
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
    // A row's CI result is its Ticket's now, as core works it out on read.
    const withCIResult = (r: NeedsMeRow) => {
      const ci_result = this.tickets.get(r.id)?.ci_result;
      return ci_result ? { ...r, ci_result } : r;
    };
    const groups = this.groups.filter((g) => g.tickets.length).map((g) => ({ ...g, tickets: g.tickets.map(withCIResult) }));
    return structuredClone({ count: this.rows().length, groups });
  }

  async findTickets(project: string, filter: FindTicketsFilter = {}): Promise<TicketRow[]> {
    this.calls.push("findTickets");
    this.project(project);
    const rows = [...this.tickets.values()]
      .filter((t) => this.projectOf(t.id) === project)
      .filter((t) => (filter.state?.length ? filter.state.includes(t.state) : !FINISHED.includes(t.state)));
    return this.rowsOf(rows);
  }

  // getTicket is the UI's read, so it carries the Model Choice.
  async getTicket(project: string, id: string): Promise<TicketView> {
    this.calls.push("getTicket");
    const t = this.ticket(project, id);
    const { models } = this.project(project);
    const name = kindOf(t.labels);
    const kind = models.kinds.find((k) => k.kind === name) ?? { model: "", effort: "" };
    return {
      ...this.view(t),
      model_choice: {
        model: choose(t.tags.model, name, kind.model, models.default.model),
        effort: choose(t.tags.effort, name, kind.effort, models.default.effort),
      },
    };
  }

  async getSpec(project: string, spec: string): Promise<SpecDetail> {
    this.calls.push("getSpec");
    this.project(project);
    const id = this.specId(project, spec);
    const s = this.allSpecs(project).find((s) => s.id === id);
    if (!s) throw new ApiError(404, "not_found", `there is no Spec ${id}`);
    const { state_summary, finished, requirement, map } = this.derive(id);
    return structuredClone({
      ...s,
      state_summary,
      finished,
      tickets: this.rowsOf(this.ticketsOf(id)),
      ...(requirement && { requirement }),
      ...(map && { map }),
    });
  }

  async findSpecs(project: string, filter: FindSpecsFilter = {}): Promise<SpecRow[]> {
    this.calls.push("findSpecs");
    this.project(project);
    return structuredClone(
      this.allSpecs(project)
        .map((s) => ({ s, ...this.derive(s.id) }))
        .filter((r) => (filter.finished || !r.finished) && (!filter.requirements || r.requirement))
        .filter((r) => (filter.labels ?? []).every((l) => r.s.labels.includes(l)))
        .map(({ s, state_summary, finished, requirement, map }) => ({
          id: s.id,
          title: s.title,
          labels: s.labels,
          tags: s.tags,
          state_summary,
          finished,
          updated: s.updated,
          ...(s.link && { link: s.link }),
          ...(requirement && { requirement }),
          ...(map && { map }),
        })),
    );
  }

  async submitRequirement(project: string, text: string, map?: string): Promise<SubmitRequirementResult> {
    this.calls.push("submitRequirement");
    this.writable(project);
    const title = text.trim().split("\n")[0].trim();
    if (!title) throw new ApiError(422, "invalid", "a Requirement needs text; its first line becomes the Spec's title", "text");
    const from = map && this.finishedMap(project, map);
    const { spec, stages } = this.addRequirement(project, { title, body: text, ...(from && { tags: { map: from } }) }, from);
    return { spec: acknowledgeSpecCreate(spec), stages: stages.map(acknowledgeTicketCreate) };
  }

  // addRequirement writes Spec seed as a Requirement with its Stage
  // Tickets: only the spec and tickets Stages when written from Map from.
  private addRequirement(project: string, seed: SeedSpec, from?: string): { spec: SpecView; stages: TicketView[] } {
    const spec = this.addSpec(project, seed);
    const title = seed.title;
    const stages: TicketView[] = [];
    for (const s of from ? STAGES.slice(1) : STAGES) {
      const prev = stages.at(-1);
      stages.push(
        this.addTicket(project, {
          title: `${s.title}: ${title}`,
          state: "Ready",
          spec: { id: spec.id, title },
          labels: [`stage:${s.name}`],
          description:
            from && s.name === "spec"
              ? `Write the Spec for the Requirement in Spec [${spec.id}](${spec.id}), from its text and the Map [${from}](${from}): its Decisions so far and the resolution comments they link.\n`
              : `The ${s.title} Stage of [${spec.id}](${spec.id}).\n`,
          blocked_by: prev ? [prev.id] : [],
          blockers: prev ? [{ id: prev.id, title: prev.title, state: prev.state, cleared: false }] : [],
          blocked: !!prev,
          claimable: !prev,
          moves: [{ to: "Ready", actor: "human", time: this.now.toISOString(), via: "submit_requirement" }],
        }),
      );
    }
    return { spec, stages };
  }

  // finishedMap is the ID of the Map submitRequirement's map names, which
  // must be a Map with no unfinished Tickets.
  private finishedMap(project: string, map: string): string {
    const id = `${project}-${numberOf(map)}`;
    const s = this.specs.get(id);
    if (!s) throw new ApiError(404, "not_found", `there is no Spec ${id}`);
    if (!s.labels.includes(MAP_LABEL)) throw new ApiError(422, "invalid", `${id} isn't a Map: a Map is a Spec labelled ${MAP_LABEL}`, "map");
    const open = this.ticketsOf(id).filter((t) => !FINISHED.includes(t.state));
    if (open.length) {
      throw new ApiError(
        409,
        "action_not_allowed",
        `Map ${id} still has unfinished Tickets, so its Spec can't be written until every one is Done or Cancelled: ${open.map((t) => `${t.id} (${t.state})`).join(", ")}`,
      );
    }
    return id;
  }

  async submitMap(project: string, text: string): Promise<SubmitMapResult> {
    this.calls.push("submitMap");
    this.writable(project);
    const title = specTitle(text);
    if (!title) throw new ApiError(422, "invalid", "a Map needs text; its first line becomes the Spec's title", "text");
    const body = text.replace(/\n+$/, "") + "\n" + MAP_SECTIONS.map((h) => `\n## ${h}\n`).join("");
    const spec = this.addSpec(project, { title, body, labels: [MAP_LABEL] });
    const charting = this.addTicket(project, {
      title: `Charting: ${title}`,
      state: "Ready",
      spec: { id: spec.id, title },
      labels: ["wayfinder:chart"],
      description: `Chart the Map [${spec.id}](${spec.id}): settle its destination with the human, fill in its sections and create its first decision Tickets.\n`,
      claimable: true,
      moves: [{ to: "Ready", actor: "human", time: this.now.toISOString(), via: "submit_map" }],
    });
    return { spec: acknowledgeSpecCreate(spec), charting: acknowledgeTicketCreate(charting) };
  }

  async submitPrototype(project: string, spec: string, text: string): Promise<TicketAcknowledgement> {
    this.calls.push("submitPrototype");
    this.writable(project);
    const title = specTitle(text);
    if (!title) throw new ApiError(422, "invalid", "a Prototype needs text; its first line becomes the Ticket's title", "text");
    const id = this.specId(project, spec);
    const on = this.allSpecs(project).find((s) => s.id === id);
    if (!on) throw new ApiError(422, "invalid", `there is no Spec ${id}`, "spec");
    const prototype = this.addTicket(project, {
      title: `Prototype: ${title}`,
      state: "Ready",
      spec: { id, title: on.title },
      labels: ["prototype"],
      description: text,
      claimable: true,
      moves: [{ to: "Ready", actor: "human", time: this.now.toISOString(), via: "submit_prototype" }],
    });
    return acknowledgeTicketCreate(prototype);
  }

  // specQuestions reads a Spec from its seeded Tickets' spec, so a Spec
  // no Ticket names is not found.
  async specQuestions(project: string, spec: string): Promise<SpecQuestions> {
    this.calls.push("specQuestions");
    this.project(project);
    const id = this.specId(project, spec);
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
    const after = handler(this, t, body);
    // A handler that returns another Ticket created it (new_fix_ticket).
    return after.id === t.id ? acknowledge(before, after) : acknowledgeTicketCreate(after);
  }

  async editSpec(project: string, spec: string, change: { title?: string; body?: string }): Promise<SpecAcknowledgement> {
    this.calls.push("editSpec");
    this.project(project);
    const id = this.specId(project, spec);
    // A Spec only a Ticket names becomes a seeded one when it is edited.
    const s = this.specs.get(id) ?? this.allSpecs(project).find((s) => s.id === id);
    if (!s) throw new ApiError(404, "not_found", `there is no Spec ${id}`);
    this.specs.set(id, s);
    if (change.title !== undefined && !change.title.trim()) throw new ApiError(422, "invalid", "a Spec needs a title", "title");
    this.writable(project);
    this.definitionOwned(id, s.link);
    const before = structuredClone(s);
    const previous: Record<string, string> = {};
    if (change.title !== undefined && change.title !== s.title) [previous.title, s.title] = [s.title, change.title];
    if (change.body !== undefined && change.body !== s.body) [previous.body, s.body] = [s.body, change.body];
    if (Object.keys(previous).length) {
      const at = this.now.toISOString();
      s.edits.push({ number: s.edits.length + 1, actor: "human", time: at, previous });
      s.updated = at;
      this.emit({ type: "spec.changed", project, id });
    }
    return written(SPEC_FIELDS, before, s, ["updated"]);
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
      // The failure options a CI failed Ticket is offered follow it.
      for (const t of this.ticketsIn(key)) if (t.pushed_commit) this.trackCI(key, t);
    }
    return this.projectView(p);
  }

  async setModels(key: string, change: Models): Promise<ProjectView> {
    this.calls.push("setModels");
    const p = this.writable(key);
    const row = (field: string, name: string, r: ModelRow): ModelRow => {
      const model = r.model.trim();
      if (r.model !== "" && model === "") {
        throw new ApiError(422, "invalid", `${name}'s model is blank; leave it empty for Default`, `${field}.model`);
      }
      if (/[\r\n]/.test(model)) {
        throw new ApiError(422, "invalid", `${name}'s model spans more than one line; a model is one line`, `${field}.model`);
      }
      const levels = p.reported_models.effort_levels;
      if (r.effort !== "" && !levels.includes(r.effort)) {
        throw new ApiError(422, "invalid", `effort "${r.effort}" isn't a valid level; use one of ${levels.join(", ")}`, `${field}.effort`);
      }
      return { model, effort: r.effort };
    };
    const kinds = new Map<string, ModelRow>();
    for (const k of change.kinds) {
      if (!KINDS_OF_WORK.includes(k.kind)) {
        throw new ApiError(422, "invalid", `"${k.kind}" isn't a kind of work; the kinds are ${KINDS_OF_WORK.join(", ")}`, "kinds");
      }
      if (kinds.has(k.kind)) throw new ApiError(422, "invalid", `there are two rows for "${k.kind}"; give each kind of work one`, "kinds");
      kinds.set(k.kind, row(`kinds.${k.kind}`, `the ${k.kind} row`, k));
    }
    const next: Models = {
      default: row("default", "the Project default", change.default),
      kinds: KINDS_OF_WORK.map((kind) => ({ kind, ...(kinds.get(kind) ?? { model: "", effort: "" }) })),
    };
    if (JSON.stringify(next) !== JSON.stringify(p.models)) {
      p.models = next;
      this.emit({ type: "project.changed", project: key });
    }
    return this.projectView(p);
  }

  async setCITracking(key: string, change: CITrackingChange): Promise<ProjectView> {
    this.calls.push("setCITracking");
    const provider = change.provider || "github_actions";
    if (provider !== "github_actions" && provider !== "azure_pipelines") {
      throw new ApiError(422, "invalid", `CI Provider "${provider}" isn't "github_actions" or "azure_pipelines"`, "provider");
    }
    const repository = change.repository.trim();
    let project_url = (change.project_url ?? "").trim();
    const pat_env = (change.pat_env ?? "").trim();
    if (provider === "azure_pipelines") {
      if (!project_url && change.tracking) {
        throw new ApiError(422, "invalid", "CI Tracking needs the Azure DevOps project to watch, as its URL", "project_url");
      }
      if (project_url) project_url = parseProjectURL(project_url);
      if (pat_env && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(pat_env)) {
        throw new ApiError(422, "invalid", `"${pat_env}" isn't an environment variable name: a letter or underscore, then letters, digits and underscores, such as ADO_PAT`, "pat_env");
      }
    } else {
      if (!repository && change.tracking) {
        throw new ApiError(422, "invalid", "CI Tracking needs the GitHub repository to watch, as owner/name", "repository");
      }
      if (repository && !/^[A-Za-z0-9-]+\/[A-Za-z0-9._-]+$/.test(repository)) {
        throw new ApiError(422, "invalid", `"${repository}" isn't a GitHub repository as owner/name, such as acme/web`, "repository");
      }
    }
    const ignored: string[] = [];
    for (const raw of change.ignored_workflows) {
      const w = raw.trim();
      if (!w || ignored.includes(w)) continue;
      const [ok, example] = checkWorkflow(provider, w);
      if (!ok) throw new ApiError(422, "invalid", `"${w}" isn't ${example}`, "ignored_workflows");
      ignored.push(w);
    }
    const p = this.writable(key);
    const patName = (e: string) => e || "ADO_PAT";
    const changed =
      !p.ci.tracking ||
      p.ci.provider !== provider ||
      p.ci.repository !== repository ||
      p.ci.project_url !== project_url ||
      patName(p.ci.pat_env) !== patName(pat_env);
    if (change.tracking && changed && this.ciCheck) {
      throw new ApiError(422, "invalid", this.ciCheck.reason, this.ciCheck.field ?? "tracking", undefined, this.ciCheck.fix);
    }
    // Turning CI Tracking off forgets the CI Watcher's status.
    const watcher = change.tracking ? p.ci.watcher : { state: "not_reported" as const };
    this.setCI(p, {
      ...p.ci,
      tracking: change.tracking,
      provider,
      repository,
      project_url,
      pat_env: patName(pat_env),
      ignored_workflows: ignored,
      unblock_on_push: change.unblock_on_push,
      watcher,
    });
    return this.projectView(p);
  }

  // reportCIWatcherStatus is the CI Watcher's report of its status
  // (code-central-253), refused while CI Tracking is off. Only the
  // service reports a crash (seedCIWatcherCrash).
  async reportCIWatcherStatus(key: string, status: CIWatcherReport): Promise<void> {
    const p = this.writable(key);
    if (!p.ci.tracking) throw new ApiError(403, "action_not_allowed", `CI Tracking is off on ${key}, so it has no CI Watcher`);
    const watcher = this.nextWatcher(p.ci.watcher, status, "CI Watcher", "CI");
    if (watcher) this.setCI(p, { ...p.ci, watcher });
  }

  // reportIssueWatcherStatus is the Issue Watcher's report of its status
  // (code-central-285), as the integration Actor, refused while GitHub
  // Issues is off. Only the service reports a crash
  // (seedIssueWatcherCrash).
  async reportIssueWatcherStatus(key: string, status: IssueWatcherReport): Promise<void> {
    const p = this.writable(key);
    if (!p.github_issues.on) throw new ApiError(403, "action_not_allowed", `GitHub Issues is off on ${key}, so it has no Issue Watcher`);
    const watcher = this.nextWatcher(p.github_issues.watcher, status, "Issue Watcher", "GitHub Issues");
    if (!watcher) return;
    p.github_issues = { ...p.github_issues, watcher };
    this.emit({ type: "project.changed", project: key });
  }

  // nextWatcher is a Watcher's status after it reports status, keeping
  // since while its state is the same; undefined when nothing changed. A
  // crash, which only the service reports, and a rate-limited error are
  // refused, naming the Watcher and what it watches.
  private nextWatcher<W extends IssueWatcherStatus>(current: W, status: Omit<W, "since">, name: string, watches: string): W | undefined {
    if (status.problem === "crashed") throw new ApiError(422, "invalid", `"crashed" isn't a ${watches} problem`, "problem");
    if (status.state === "error" && status.rate_limited_until)
      throw new ApiError(422, "invalid", `only a Watching ${name} is rate-limited; an error has its own fix`, "rate_limited_until");
    const watcher = (
      status.state === "error"
        ? { state: "error", problem: status.problem, message: status.message, fix: status.fix }
        : status.rate_limited_until
          ? { state: status.state, rate_limited_until: status.rate_limited_until }
          : { state: status.state }
    ) as W;
    const { since: _, ...was } = current;
    if (JSON.stringify(was) === JSON.stringify(watcher)) return undefined;
    watcher.since = was.state === watcher.state ? current.since : this.now.toISOString();
    return watcher;
  }

  // reportPushRemote is the Factory's report of its Push remote's URL,
  // made when it starts, kept without an HTTP(S) URL's login.
  async reportPushRemote(key: string, url: string): Promise<void> {
    const push_remote_url = url.trim().replace(/^(https?:\/\/)[^@/]+@/, "$1");
    this.setCI(this.writable(key), { ...this.project(key).ci, push_remote_url });
  }

  // reportModels is the Factory's startup report of what its claude
  // accepts, as the service keeps it: empty levels or aliases keep the
  // kept ones, the flags are always replaced, and model IDs stay.
  async reportModels(key: string, report: ModelsReport): Promise<void> {
    const oneLine = (field: string, v: string) => {
      if (/[\r\n]/.test(v)) throw new ApiError(422, "invalid", `"${v}" spans more than one line; each is one line`, field);
      return v;
    };
    const list = (field: string, vs: string[]) => [...new Set(vs.map((v) => oneLine(field, v.trim())).filter((v) => v !== ""))];
    const levels = list("effort_levels", report.effort_levels);
    const aliases = list("aliases", report.aliases);
    const model_flag = oneLine("model_flag", report.model_flag.trim());
    const effort_flag = oneLine("effort_flag", report.effort_flag.trim());
    const p = this.writable(key);
    const was = p.reported_models;
    p.reported_models = {
      ...was,
      effort_levels: levels.length ? levels : was.effort_levels,
      aliases: aliases.length ? aliases : was.aliases,
      model_flag,
      effort_flag,
      reported: this.now.toISOString(),
    };
    this.emit({ type: "project.changed", project: key });
  }

  // reportHeartbeat records a Factory's Heartbeat as Code Central does,
  // without the clock: the status echoes the Heartbeat's state, a stopping
  // one being offline, and time passing never changes it.
  async reportHeartbeat(key: string, report: HeartbeatReport): Promise<void> {
    const name = report.factory.trim();
    const host = report.host.trim();
    if (!name) throw new ApiError(422, "invalid", "factory is required", "factory");
    if (!host) throw new ApiError(422, "invalid", "host is required", "host");
    const p = this.writable(key);
    const factory = {
      name,
      host,
      status: report.state === "stopping" ? ("offline" as const) : report.state,
      note: (report.note ?? "").trim().slice(0, 200),
      last_heartbeat: this.now.toISOString(),
      interval_seconds: report.interval_seconds,
    };
    const factories = [...p.factory_status.factories.filter((f) => f.name !== name || f.host !== host), factory].sort(
      (a, b) => a.name.localeCompare(b.name) || a.host.localeCompare(b.host),
    );
    const status = factories.some((f) => f.status === "online") ? "online" : factories.some((f) => f.status === "draining") ? "draining" : "offline";
    p.factory_status = { status, factories };
    this.emit({ type: "project.changed", project: key });
  }

  async setGitHubIssues(key: string, change: GitHubIssuesChange): Promise<ProjectView> {
    this.calls.push("setGitHubIssues");
    const repository = change.repository.trim();
    if (!repository && change.on) {
      throw new ApiError(422, "invalid", "GitHub Issues needs the GitHub repository its issues come from, as owner/name", "repository");
    }
    if (repository && !/^[A-Za-z0-9-]+\/[A-Za-z0-9._-]+$/.test(repository)) {
      throw new ApiError(422, "invalid", `"${repository}" isn't a GitHub repository as owner/name, such as acme/web`, "repository");
    }
    const p = this.writable(key);
    const was = p.github_issues;
    if (change.on && !(was.on && was.repository === repository) && this.issuesCheck) {
      throw new ApiError(422, "invalid", this.issuesCheck.reason, this.issuesCheck.field ?? "on", undefined, this.issuesCheck.fix);
    }
    let spec = was.spec;
    if (change.on && !spec) spec = this.addSpec(key, { title: issuesSpecTitle(repository), body: ISSUES_SPEC_BODY }).id;
    // Turning GitHub Issues off, or changing its repository, forgets the
    // Issue Watcher's status.
    const watcher: IssueWatcherStatus = change.on && was.repository === repository ? was.watcher : { state: "not_reported" };
    const next: GitHubIssuesSettings = {
      on: change.on,
      repository,
      label: change.label.trim(),
      new_issues_need_me: change.new_issues_need_me,
      ...(spec && { spec }),
      watcher,
    };
    if (JSON.stringify(next) !== JSON.stringify(was)) {
      p.github_issues = next;
      this.emit({ type: "project.changed", project: key });
    }
    // The Issues Spec's title follows the repository, unless the human
    // retitled it: it still has the title the service gave it.
    const s = was.spec && this.specs.get(was.spec);
    const given = s && /^GitHub Issues: [A-Za-z0-9-]+\/[A-Za-z0-9._-]+$/.test(s.title);
    if (s && given && repository && was.repository !== repository && s.title !== issuesSpecTitle(repository)) {
      s.edits.push({ number: s.edits.length + 1, actor: "human", time: this.now.toISOString(), previous: { title: s.title } });
      s.title = issuesSpecTitle(repository);
      s.updated = this.now.toISOString();
      this.emit({ type: "spec.changed", project: key, id: s.id });
    }
    return this.projectView(p);
  }

  async addCIRule(key: string, change: CIRuleChange): Promise<ProjectView> {
    this.calls.push("addCIRule");
    const p = this.writable(key);
    const rule = checkCIRule(p, change, "");
    const at = this.now.toISOString();
    const spec = this.addSpec(key, { title: ciRuleTitle(rule), body: CI_RULE_SPEC_BODY });
    this.setCI(p, { ...p.ci, rules: [...p.ci.rules, { ...rule, spec: spec.id, created: at }] });
    return this.projectView(p);
  }

  async editCIRule(key: string, spec: string, change: CIRuleChange): Promise<ProjectView> {
    this.calls.push("editCIRule");
    const p = this.writable(key);
    const old = ruleOn(p, spec);
    const rule = { ...old, ...checkCIRule(p, change, spec) };
    this.setCI(p, { ...p.ci, rules: p.ci.rules.map((r) => (r.spec === spec ? rule : r)) });
    // The rule's Spec's title follows it, unless the human retitled it.
    const s = this.specs.get(spec);
    if (s && s.title === ciRuleTitle(old) && ciRuleTitle(old) !== ciRuleTitle(rule)) {
      s.edits.push({ number: s.edits.length + 1, actor: "human", time: this.now.toISOString(), previous: { title: s.title } });
      s.title = ciRuleTitle(rule);
      s.updated = this.now.toISOString();
      this.emit({ type: "spec.changed", project: key, id: spec });
    }
    return this.projectView(p);
  }

  async removeCIRule(key: string, spec: string): Promise<ProjectView> {
    this.calls.push("removeCIRule");
    const p = this.writable(key);
    ruleOn(p, spec);
    this.setCI(p, { ...p.ci, rules: p.ci.rules.filter((r) => r.spec !== spec) });
    return this.projectView(p);
  }

  // reportPushedCommit is the Factory's report of the commit it pushed for
  // an Accepted or In Review Ticket (code-central-256), tracked only while
  // CI Tracking and Push are on.
  async reportPushedCommit(key: string, id: string, commit: string): Promise<void> {
    const p = this.writable(key);
    const t = this.ticket(key, id);
    if (t.state !== "Accepted" && t.state !== "In Review") {
      throw new ApiError(403, "action_not_allowed", `${t.id} is ${t.state}: only an Accepted or In Review Ticket is pushed`);
    }
    const pushed = this.pushedCommits.get(t.id) ?? [];
    if (!p.ci.tracking || !p.landing.push || pushed.at(-1)?.commit === commit) return;
    this.pushedCommits.set(t.id, [...pushed, { commit, pushed: this.now.toISOString() }]);
    this.trackCI(key, t);
  }

  // reportCIRuns is the CI Watcher's report of CI Runs (code-central-256),
  // upserted by run ID and attempt, a run of an Ignored workflow marked
  // ignored; refused while CI Tracking is off.
  async reportCIRuns(key: string, runs: Omit<CIRunView, "ignored">[]): Promise<void> {
    const p = this.writable(key);
    if (!p.ci.tracking) throw new ApiError(403, "action_not_allowed", `CI Tracking is off on ${key}, so it tracks no CI Runs`);
    runs.forEach((r, i) => {
      const field = `runs[${i}]`;
      if (!["queued", "in_progress", "completed"].includes(r.status)) {
        throw new ApiError(422, "invalid", `"${r.status}" isn't a CI Run status`, `${field}.status`);
      }
      if ((r.status === "completed") !== (r.conclusion !== undefined)) {
        throw new ApiError(422, "invalid", "a completed CI Run has a conclusion, and an unfinished one has none", `${field}.conclusion`);
      }
      if ((r.ticket === undefined) === (r.rule === undefined)) {
        throw new ApiError(422, "invalid", "a CI Run is tracked against one Ticket or one CI Rule", `${field}.ticket`);
      }
      if (r.ticket !== undefined) this.ticket(key, r.ticket);
    });
    const all = this.ciRuns.get(key) ?? [];
    for (const r of runs) {
      const run = { ...structuredClone(r), ignored: p.ci.ignored_workflows.includes(r.workflow) };
      const at = all.findIndex((x) => x.id === run.id && x.attempt === run.attempt);
      if (at < 0) all.push(run);
      else all[at] = run;
    }
    this.ciRuns.set(key, all);
    this.emit({ type: "ci.changed", project: key });
    for (const id of new Set(runs.map((r) => r.ticket).filter((t) => t !== undefined))) this.trackCI(key, this.ticket(key, id));
  }

  // trackCI works t's pushed commit, CI Runs and CI result out, as core's
  // ticketCI does on read.
  private trackCI(key: string, t: TicketView): TicketView {
    const pushed = this.pushedCommits.get(t.id) ?? [];
    const latest = pushed.at(-1);
    if (!latest) return this.view(t);
    const order = (commit: string) => pushed.length - 1 - pushed.findIndex((c) => c.commit === commit);
    const mine = (this.ciRuns.get(key) ?? []).filter((r) => r.ticket === t.id).sort((a, b) => order(a.commit) - order(b.commit));
    const finished = this.countedRuns(key, t.id, latest.commit);
    // A Re-run request waiting on the CI Watcher reads as running.
    const rerunning = this.pendingReruns(key, t.id, latest.commit).length > 0;
    const ci_result: CIResult =
      rerunning || finished.length === 0 || finished.some((r) => r.status !== "completed")
        ? "running"
        : finished.some((r) => r.conclusion !== undefined && FAILED_CONCLUSIONS.includes(r.conclusion))
          ? "failed"
          : finished.some((r) => r.conclusion === "cancelled")
            ? "cancelled"
            : "passed";
    // CI failed: Accepted, failed, and the human hasn't chosen Land anyway;
    // Land anyway is offered only under Merge, Re-run failed jobs and the
    // two fixes under both.
    const land_anyway = this.landedAnyway.has(`${t.id}@${latest.commit}`);
    // CI cancelled is offered the same without New fix Ticket.
    const ciFailed = t.state === "Accepted" && (ci_result === "failed" || ci_result === "cancelled") && !land_anyway;
    const fixes: CIFailureOption[] = ci_result === "cancelled" ? ["send_back"] : ["send_back", "new_fix_ticket"];
    const options: CIFailureOption[] = this.project(key).landing.method === "merge" ? ["rerun_ci", "land_anyway", ...fixes] : ["rerun_ci", ...fixes];
    const { land_anyway: _l, ci_failure_options: _o, ...rest } = this.tickets.get(t.id)!;
    this.tickets.set(t.id, rest);
    return this.updateTicket(t.id, {
      pushed_commit: latest,
      ci_result,
      ...(mine.length > 0 && { ci_runs: structuredClone(mine) }),
      ...(land_anyway && { land_anyway }),
      ...(ciFailed && { ci_failure_options: options }),
    });
  }

  private setCI(p: FakeProject, ci: CISettings) {
    if (JSON.stringify(p.ci) === JSON.stringify(ci)) return;
    p.ci = ci;
    this.emit({ type: "project.changed", project: p.key });
  }

  // unlink detaches a seeded Ticket's Link, taking it out of Needs Me if it
  // was there for its Link (closed externally or New issue); detached, it
  // is no longer an Incoming Issue, so it is offered no triage action.
  async unlink(project: string, target: string): Promise<LinkedRow> {
    this.calls.push("unlink");
    const t = this.ticket(project, target);
    this.writable(project);
    if (!t.link) throw new ApiError(422, "invalid", `${t.id} is not linked`, "target");
    const reason = this.rows().find((r) => r.id === t.id)?.reason;
    if (reason === "closed_externally" || reason === "new_issue") this.dropNeedsMe(t.id);
    const { triage_actions: _t, ...rest } = this.tickets.get(t.id)!;
    this.tickets.set(t.id, rest);
    const v = this.updateTicket(t.id, { link: { ...t.link, detached: true } });
    return { kind: "ticket", id: v.id, title: v.title, state: v.state, link: v.link! };
  }

  async setTags(project: string, target: string, tags: Record<string, string>): Promise<TargetView> {
    this.calls.push("setTags");
    // On a Ticket, effort sets its Model Choice, so it is checked.
    const levels = this.project(project).reported_models.effort_levels;
    if (tags.effort !== undefined && !this.specs.has(this.specId(project, target)) && !levels.includes(tags.effort)) {
      throw new ApiError(422, "invalid", `effort "${tags.effort}" isn't a valid level; use one of ${levels.join(", ")}`, "tags");
    }
    return this.retag(project, target, (old) => ({ ...old, ...tags }));
  }

  async removeTags(project: string, target: string, keys: string[]): Promise<TargetView> {
    this.calls.push("removeTags");
    return this.retag(project, target, (old) => Object.fromEntries(Object.entries(old).filter(([k]) => !keys.includes(k))));
  }

  // retag replaces a Spec's or Ticket's Tags with change's.
  private retag(project: string, target: string, change: (old: Record<string, string>) => Record<string, string>): TargetView {
    this.writable(project);
    const id = `${project}-${target.slice(target.lastIndexOf("-") + 1)}`;
    const s = this.specs.get(id);
    if (s) {
      Object.assign(s, { tags: change(s.tags), updated: this.now.toISOString() });
      this.emit({ type: "spec.changed", project, id });
      return structuredClone({ id, kind: "spec", labels: s.labels, tags: s.tags });
    }
    const old = this.ticket(project, target);
    const t = this.updateTicket(old.id, { tags: change(old.tags) });
    return structuredClone({ id: t.id, kind: "ticket", labels: t.labels, tags: t.tags });
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
    if (change.gh_path !== undefined) {
      const gh_path = change.gh_path.trim();
      if (gh_path && !/^(\/|[A-Za-z]:[\\/]|\\\\)/.test(gh_path)) {
        throw new ApiError(422, "invalid", `gh path "${gh_path}" isn't an absolute path, such as /opt/homebrew/bin/gh`, "gh_path");
      }
      change = { ...change, gh_path };
    }
    const next = { ...this.opts, ...change };
    if (Object.keys(DEFAULT_OPTIONS).some((k) => next[k as keyof Options] !== this.opts[k as keyof Options])) {
      this.opts = next;
      this.emit({ type: "options.changed" });
    }
    return { ...this.opts };
  }

  // ciOverview reads the CI view as core's CIOverview does: each run's latest
  // attempt, unfinished ones first, newest started first, then those
  // finished in the last CI_VIEW_DAYS days, newest finished first. With
  // running, only the unfinished runs that count towards their Ticket's CI
  // result: tracked against a Ticket, on its latest pushed commit, not ignored.
  async ciOverview(filter: CIOverviewFilter = {}): Promise<CIOverview> {
    this.calls.push("ciOverview");
    if (filter.project && !this.projects.has(filter.project)) throw new ApiError(404, "not_found", `no Project "${filter.project}"`);
    const projects = filter.project ? [this.project(filter.project)] : [...this.projects.values()].filter((p) => p.ci.tracking && !p.archived);
    const ended = (r: CIRunView) => (r.status === "completed" ? (r.finished ?? r.started) : undefined);
    const since = this.now.getTime() - CI_VIEW_DAYS * 24 * 60 * 60 * 1000;
    const runs: CIRunRow[] = projects.flatMap((p) => {
      const all = this.ciRuns.get(p.key) ?? [];
      const latest = all.filter((r) => !all.some((o) => o.id === r.id && o.attempt > r.attempt));
      const running = (r: CIRunView) =>
        r.status !== "completed" && r.ticket !== undefined && !r.ignored && r.commit === this.pushedCommits.get(r.ticket)?.at(-1)?.commit;
      return latest
        .filter((r) => (filter.running ? running(r) : r.status !== "completed" || (ended(r) !== undefined && Date.parse(ended(r)!) >= since)))
        .map((r) => ({ ...structuredClone(r), project: p.key }));
    });
    const time = (at?: string) => (at ? Date.parse(at) : 0);
    runs.sort((a, b) => {
      const [x, y] = [ended(a), ended(b)];
      if ((x === undefined) !== (y === undefined)) return x === undefined ? -1 : 1;
      return x === undefined ? time(b.started) - time(a.started) : time(y) - time(x);
    });
    return {
      projects: projects.map((p) => ({ project: p.key, tracking: p.ci.tracking, provider: p.ci.provider, repository: p.ci.repository, project_url: p.ci.project_url, watcher: structuredClone(p.ci.watcher) })),
      runs,
    };
  }

  async usage(filter: UsageFilter = {}): Promise<UsageView> {
    this.calls.push("usage");
    const days = filter.days ?? USAGE_DAYS[0];
    if (!USAGE_DAYS.includes(days)) throw new ApiError(422, "invalid", `days must be 7, 30 or 90, not ${days}`, "days");
    if (filter.project && !this.projects.has(filter.project)) throw new ApiError(404, "not_found", `no Project "${filter.project}"`);
    const inRange = (r: { project: string; time: string }) => (!filter.project || r.project === filter.project) && this.withinDays(r.time, days);
    const rows = new Map<string, UsageOperation>();
    for (const c of this.usageCalls) {
      if (!inRange(c)) continue;
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
    const responses = this.usageResponses.filter(inRange);
    const sessions = usageSessions(responses);
    return { operations, kinds: usageKinds(sessions), sessions: sessions.slice(0, RECENT_SESSIONS), days: usageDays(this.now, days, responses) };
  }

  async sessionUsage({ project, session, agent }: SessionUsageFilter): Promise<SessionUsageView> {
    this.calls.push("sessionUsage");
    if (!this.projects.has(project)) throw new ApiError(404, "not_found", `no Project "${project}"`);
    const key = sessionKey({ project, session, agent });
    const responses = this.usageResponses
      // Every day Usage keeps, whatever range the rest of Usage reads.
      .filter((r) => sessionKey({ project: r.project, session: r.session_id, agent: r.agent }) === key && this.withinDays(r.time, USAGE_KEPT_DAYS + 1))
      .sort((a, b) => Date.parse(a.time) - Date.parse(b.time))
      .map((r) => ({
        time: r.time,
        context_size: contextSize(r),
        input_tokens: r.input_tokens,
        cache_write_tokens: r.cache_write_tokens,
        cache_read_tokens: r.cache_read_tokens,
        output_tokens: r.output_tokens,
      }));
    return { responses };
  }

  // ---- internals ----

  // withinDays reports whether time falls on today or the days-1 local days before it.
  private withinDays(time: string, days: number): boolean {
    const ago = localDay(this.now) - localDay(new Date(time));
    return ago >= 0 && ago < days;
  }

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
    const onMap = (t: TicketView) => !!this.specs.get(t.spec.id)?.labels.includes(MAP_LABEL);
    return tickets.map((t) => ({
      id: t.id,
      title: t.title,
      state: t.state,
      spec: t.spec.id,
      labels: [...t.labels],
      blocked: t.blocked,
      updated: t.updated,
      ...(t.link && { link: { ...t.link } }),
      ...(onMap(t) && { node_type: nodeTypeOf(t), blocked_by: [...t.blocked_by] }),
      ...this.liveLatest(t),
      ...(t.ci_result && { ci_result: t.ci_result }),
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
  private derive(spec: string): { state_summary: StateSummary; finished: boolean; requirement?: RequirementView; map?: MapView } {
    const tickets = this.ticketsOf(spec);
    const state_summary: StateSummary = {};
    for (const t of tickets) state_summary[t.state] = (state_summary[t.state] ?? 0) + 1;
    const finished = tickets.length > 0 && tickets.every((t) => FINISHED.includes(t.state));
    const reason = (t: TicketView) => this.rows().find((r) => r.id === t.id)?.reason;
    if (this.specs.get(spec)?.labels.includes(MAP_LABEL)) {
      const wayfinder = tickets.filter((t) => nodeTypeOf(t) !== "prototype" && nodeTypeOf(t) !== "ticket" && t.state !== "Cancelled");
      return {
        state_summary,
        finished,
        map: {
          decided: wayfinder.filter((t) => t.state === "Done").length,
          total: wayfinder.length,
          needs_you: tickets.filter(reason).length,
          guided: this.specs.get(spec)?.tags[GUIDED_TAG] === GUIDED_ON,
          can_write_spec: tickets.every((t) => FINISHED.includes(t.state)),
          requirements: [...this.specs.values()].filter((s) => s.tags.map === spec).map((s) => s.id),
        },
      };
    }
    const stages = STAGES.map((s) => tickets.find((t) => stageOf(t) === s.name)).filter((t) => !!t);
    if (!stages.length) return { state_summary, finished };
    // The Map a Requirement was made from, when it has no grilling Stage.
    const map = stageOf(stages[0]) !== "grilling" ? this.specs.get(spec)?.tags.map : undefined;
    // A Ticket a Factory could claim now; one waiting on the human doesn't count.
    const claimable = (t: TicketView) =>
      t.state === "Ready" && !reason(t) && t.blocked_by.every((b) => this.tickets.get(b)?.state === "Done");
    const build = tickets.filter((t) => !stageOf(t) && t.state !== "Cancelled");
    const current = stages.find((t) => t.state !== "Done" && t.state !== "Cancelled");
    const held = tickets.some((t) => t.claims.some((c) => !c.ended));
    return {
      state_summary,
      finished,
      requirement: {
        ...(map && { map }),
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

  // specId is the display ID of spec, given as an ID or a number, in project.
  private specId(project: string, spec: string) {
    return `${project}-${spec.slice(spec.lastIndexOf("-") + 1)}`;
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
    // Only an Accepted Ticket is CI failed, as the service works it out on
    // every read.
    if (v.state !== "Accepted") delete v.ci_failure_options;
    return v;
  }
}

// ---- acknowledgements ----

// written is a write's acknowledgement, as the service derives it: the
// fields, in the view's order, whose value differs between the views
// before and after the write, except those unlisted.
function written<V extends SpecView | TicketView, F extends keyof V & string>(fields: readonly F[], before: V, after: V, unlisted: string[]) {
  const changed = fields.filter((f) => !unlisted.includes(f) && JSON.stringify(before[f]) !== JSON.stringify(after[f]));
  const edit = (changed as string[]).includes("edits") ? after.edits.at(-1)?.number : undefined;
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

// checkCIRule is change trimmed, or the service's refusal: a workflow
// file name or any, an exact branch, Draft or Ready, and no other rule
// than the one on Spec self watching the same workflow and branch.
function checkCIRule(p: FakeProject, change: CIRuleChange, self: string): CIRuleChange {
  const rule: CIRuleChange = { workflow: change.workflow.trim(), branch: change.branch.trim(), state: change.state };
  const [ok, example] = checkWorkflow(p.ci.provider, rule.workflow);
  if (rule.workflow !== "any" && !ok) {
    throw new ApiError(422, "invalid", `"${rule.workflow}" isn't ${example}, or any for any workflow`, "workflow");
  }
  if (!rule.branch || /[\s*?[\]]/.test(rule.branch)) {
    throw new ApiError(422, "invalid", `"${rule.branch}" isn't an exact branch name, such as main: a CI Rule's branch takes no globs`, "branch");
  }
  if (rule.state !== "Draft" && rule.state !== "Ready") {
    throw new ApiError(422, "invalid", `a CI Rule's Tickets open as Draft or Ready, not "${rule.state}"`, "state");
  }
  const other = p.ci.rules.find((r) => r.spec !== self && r.workflow === rule.workflow && r.branch === rule.branch);
  if (other) throw new ApiError(422, "invalid", `the CI Rule on Spec ${other.spec} already watches ${ciRuleWatches(rule)}`, "workflow");
  return rule;
}

// ruleOn is the rule on Spec spec, or the service's not_found.
function ruleOn(p: FakeProject, spec: string): CIRule {
  const r = p.ci.rules.find((r) => r.spec === spec);
  if (!r) throw new ApiError(404, "not_found", `there is no CI Rule on Spec ${spec}`);
  return r;
}

// The service's body for a CI Rule's Spec.
const CI_RULE_SPEC_BODY =
  "This Spec holds the fix Tickets of a CI Rule, which opens a Ticket to fix its workflow when it fails on its branch. " +
  "The rule is in the CI section of Project Options; removing it leaves this Spec as the record of its Tickets.\n";

// issuesSpecTitle is the Issues Spec's title, as the service gives it.
const issuesSpecTitle = (repository: string) => `GitHub Issues: ${repository}`;

// The service's body for the Issues Spec.
const ISSUES_SPEC_BODY =
  "This Spec holds the Project's Incoming Issues: each open GitHub issue arrives on it as a linked Draft Ticket, to triage with Keep as Draft, To Requirement or Won't fix. " +
  "GitHub Issues is in the GitHub Issues section of Project Options; turning it off leaves this Spec and its Tickets as they are.\n";

// WORKFLOW is a workflow file's base name, holding no glob.
const WORKFLOW = /^[^/\\\s*?[\]]+\.ya?ml$/;

// checkWorkflow is whether name is a workflow name for provider, with an
// example of one: a workflow file for GitHub Actions, any non-empty
// pipeline name without a separator or glob, spaces allowed, for Azure
// Pipelines.
function checkWorkflow(provider: CIProvider, name: string): [boolean, string] {
  if (provider === "azure_pipelines") return [name !== "" && !/[/\\*?[\]]/.test(name), "a pipeline's name, such as Build"];
  return [WORKFLOW.test(name), "a workflow file name, such as ci.yml"];
}

// parseProjectURL is the stored project URL for what the human typed, or
// the service's refusal, as azdourl's ParseProjectURL does.
function parseProjectURL(text: string): string {
  const bad = (why: string) =>
    new ApiError(422, "invalid", `"${text}" is not an Azure DevOps project URL: ${why}, such as https://dev.azure.com/acme/web`, "project_url");
  let u: URL;
  try {
    u = new URL(text.replace(/ /g, "%20"));
  } catch {
    throw bad("it can't be read as a URL");
  }
  if (u.protocol !== "https:" && u.protocol !== "http:") throw bad("it must start with https://");
  let segs = u.pathname.replace(/\/$/, "").split("/").slice(1);
  if (segs.length === 1 && segs[0] === "") segs = [];
  const cut = segs.findIndex((s) => s.startsWith("_"));
  if (cut >= 0) segs = segs.slice(0, cut);
  if (segs.some((s) => s === "")) throw bad("its path has an empty part");
  const host = u.hostname.toLowerCase();
  if (host === "dev.azure.com") {
    if (segs.length !== 2) throw bad("on dev.azure.com it is /<organization>/<project>");
  } else if (host.endsWith(".visualstudio.com")) {
    if (!(segs.length === 1 || (segs.length === 2 && segs[0].toLowerCase() === "defaultcollection"))) {
      throw bad("on visualstudio.com it is /<project> or /DefaultCollection/<project>");
    }
  } else if (segs.length < 2) {
    throw bad("on Azure DevOps Server it is /<collection>/<project>");
  }
  return `${u.protocol}//${u.host.toLowerCase()}/${segs.join("/")}`;
}

const ciRuleWatches = (r: CIRuleChange) => `${r.workflow === "any" ? "any workflow" : r.workflow} on ${r.branch}`;
const ciRuleTitle = (r: CIRuleChange) => `CI: ${ciRuleWatches(r)}`;
