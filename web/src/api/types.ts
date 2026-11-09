// The wire types: the JSON the HTTP API returns, mirroring core's view
// types (core/*.go, snake_case tags). Times are RFC 3339 strings.

export const STATES = [
  "Draft",
  "Ready",
  "In Progress",
  "Needs Answer",
  "Answered",
  "Failed",
  "In Review",
  "Accepted",
  "Done",
  "Cancelled",
] as const;
export type State = (typeof STATES)[number];

// A finished Ticket is Done or Cancelled.
export const FINISHED: readonly State[] = ["Done", "Cancelled"];

export type NeedsMeReason =
  | "needs_answer"
  | "answered"
  | "silent_claim"
  | "long_claim"
  | "failed"
  | "in_review"
  | "to_merge"
  | "accepted_not_landed"
  | "blocked_by_cancelled"
  | "closed_externally";

// A Project's mode: where init puts its files.
export type Mode = "committed" | "local";

// A Landing Method: how a Project's accepted work reaches its base.
export type LandingMethod = "merge" | "branch";

// A Project's Landing Method with its Push option and the remote it uses.
export interface Landing {
  method: LandingMethod;
  push: boolean;
  remote: string;
}

export interface ProjectView {
  key: string;
  display_name: string;
  created: string;
  mode: Mode;
  archived: boolean;
  // archived_at is when the human archived the Project, while it is.
  archived_at?: string;
  // landing is always filled in: merge, push off, origin by default.
  landing: Landing;
}

export interface ProjectRow extends ProjectView {
  needs_me: number;
  // unfinished counts the Tickets not Done or Cancelled; Archive waits for 0.
  unfinished: number;
}

export type ExternalState = "open" | "closed" | "deleted" | "out_of_scope";

// A linked Spec's or Ticket's Link (ticket 42): the external item its
// Integration mirrors. While it isn't detached, the title and description
// belong to the external system.
export interface LinkView {
  integration: string;
  id: string;
  url?: string;
  external_state: ExternalState;
  detached: boolean;
}

// unlink's result: the Spec or Ticket with its now detached Link.
export interface LinkedRow {
  kind: "spec" | "ticket";
  id: string;
  title: string;
  state?: State;
  link: LinkView;
}

export interface BlockerView {
  id: string;
  title: string;
  state: State;
  via?: string;
}

export interface NeedsMeRow {
  project: string;
  id: string;
  title: string;
  state: State;
  reason: NeedsMeReason;
  since: string;
  round?: { number: number; open: number; questions: number };
  failure?: { headline: string; reports: number };
  summary?: string;
  // branch is the Ticket's branch, for the human to merge (to_merge).
  branch?: string;
  blocker?: BlockerView;
  link?: LinkView;
  // Set when the Ticket's Spec is a Requirement.
  requirement?: NeedsMeRequirement;
}

// The Requirement a Needs Me row's Ticket is on; stage is set only on a
// Stage Ticket.
export interface NeedsMeRequirement {
  spec: string;
  title: string;
  stage?: StageName;
}

export type StageName = "grilling" | "spec" | "tickets";

// A Spec read's requirement block, present only on a Requirement (a Spec
// with Stage Tickets). Nothing in it is stored.
export interface RequirementView {
  stages: StageView[];
  build: { done: number; total: number };
  waiting_for_factory: boolean;
}

export interface StageView {
  name: StageName;
  ticket: string;
  state: State;
  needs_me_reason?: NeedsMeReason;
}

// A Spec read's Map block, present only on a Map (a Spec labelled
// wayfinder:map). Nothing in it is stored.
export interface MapView {
  // decided is the Map's Wayfinder Tickets that are Done.
  decided: number;
  // total is its Wayfinder Tickets that aren't Cancelled.
  total: number;
  // needs_you is its Tickets with a Needs Me reason, Prototypes included.
  needs_you: number;
  // can_write_spec is whether it has no unfinished Tickets.
  can_write_spec: boolean;
}

// A Map Ticket's type, from its reserved Label; "ticket" when it has none.
export type NodeType = "chart" | "grilling" | "research" | "task" | "prototype" | "ticket";

// MAP_LABEL marks a Spec as a Map.
export const MAP_LABEL = "wayfinder:map";

// A Spec as get_spec and submit_requirement return it.
export interface SpecView {
  id: string;
  title: string;
  labels: string[];
  tags: Record<string, string>;
  created: string;
  updated: string;
  body: string;
  edits: EditView[];
  link?: LinkView;
}

// How many of a Spec's Tickets are in each State; States with none are left out.
export type StateSummary = Partial<Record<State, number>>;

// get_spec: the Spec, its Tickets (finished ones too) in claim order, and
// its requirement block when it is a Requirement, or Map block when a Map.
export interface SpecDetail extends SpecView {
  state_summary: StateSummary;
  finished: boolean;
  tickets: TicketRow[];
  requirement?: RequirementView;
  map?: MapView;
}

// One find_specs result.
export interface SpecRow {
  id: string;
  title: string;
  labels: string[];
  tags: Record<string, string>;
  state_summary: StateSummary;
  finished: boolean;
  updated: string;
  link?: LinkView;
  requirement?: RequirementView;
  map?: MapView;
}

export interface FindSpecsFilter {
  // requirements keeps only Requirements.
  requirements?: boolean;
  // labels keeps only Specs with every one of these Labels.
  labels?: string[];
  // finished includes finished Specs, which are otherwise left out.
  finished?: boolean;
}

// submit_requirement's result (ADR 0009): the acknowledgements of the
// new Spec and its three Stage Tickets, in order.
export interface SubmitRequirementResult {
  spec: SpecAcknowledgement;
  stages: TicketAcknowledgement[];
}

// submit_map's result: the acknowledgements of the new Map's Spec and its
// Charting Ticket.
export interface SubmitMapResult {
  spec: SpecAcknowledgement;
  charting: TicketAcknowledgement;
}

export interface NeedsMeGroup {
  reason: NeedsMeReason;
  tickets: NeedsMeRow[];
}

export interface NeedsMeView {
  count: number;
  groups: NeedsMeGroup[];
}

export interface TicketRow {
  id: string;
  title: string;
  state: State;
  spec: string;
  labels: string[];
  blocked: boolean;
  updated: string;
  link?: LinkView;
  // node_type is the Ticket's type on a Map; left out on any other Spec.
  node_type?: NodeType;
  // latest_event is the latest Session Event of the Ticket's live Claim;
  // left out when there is none.
  latest_event?: LatestEvent;
}

export interface FindTicketsFilter {
  state?: State[];
}

export interface QuestionView {
  number: number;
  title: string;
  body: string;
  recommended?: string;
  status: "open" | "answered" | "dismissed";
  answer?: string;
  closed?: string;
}

export interface RoundView {
  number: number;
  claim: number;
  session_id?: string;
  posted: string;
  questions: QuestionView[];
}

// The roll-up of a Spec's Questions (get_spec_questions): each of its
// Tickets that has Rounds, in claim order.
export interface SpecQuestions {
  spec: string;
  tickets: { id: string; title: string; state: State; rounds: RoundView[] }[];
}

export interface ClaimView {
  number: number;
  session_id?: string;
  // As the holder last set them; left out when unset, never a default.
  working_dir?: string;
  resume_command?: string;
  started: string;
  last_activity: string;
  ended?: string;
  ended_by?: string;
  // latest_event is the Claim's latest Session Event, shown short; left
  // out when the Claim has none.
  latest_event?: LatestEvent;
}

export type SessionEventKind = "text" | "tool_call" | "tool_result" | "factory";

// A Session Event as a Factory posts it (POST /api/session-events): one
// thing a Claim's session did. Code Central treats tool and text as
// opaque strings.
export interface SessionEvent {
  time: string;
  kind: SessionEventKind;
  text: string;
  tool?: string;
  tool_use_id?: string;
  is_error?: boolean;
  truncated?: boolean;
}

// A Session Event as a Claim's Transcript holds it: seq rises within the
// Transcript, and is the SSE id its stream resumes from.
export interface HeldSessionEvent extends SessionEvent {
  seq: number;
}

// A Claim's Transcript (GET .../claims/{c}/transcript): the held events,
// oldest first. tail is true when it holds only part of the session:
// Code Central keeps the last 2,000 and none from before its last restart.
export interface Transcript {
  events: HeldSessionEvent[];
  tail: boolean;
}

// A Claim's latest Session Event, shown short: text is one line, cut as
// core's LatestEventLimit says. Computed on read, never stored.
export interface LatestEvent {
  kind: SessionEventKind;
  text: string;
  time: string;
}

export interface FailureReportView {
  number: number;
  claim: number;
  // Copies of the Claim's, made when the report was written.
  session_id?: string;
  working_dir?: string;
  resume_command?: string;
  actor: string;
  time: string;
  what_broke: string;
  why: string;
  what_to_fix: string;
  where_work_stands: string;
}

export interface CommentView {
  number: number;
  actor: string;
  time: string;
  claim?: number;
  body: string;
}

export interface MoveView {
  from?: State;
  to: State;
  actor: string;
  time: string;
  claim?: number;
  via?: string;
  // commit is the commit that put the work into the base, on a land.
  commit?: string;
}

export interface EditView {
  number: number;
  actor: string;
  time: string;
  previous: Record<string, string>;
}

export interface TicketView {
  id: string;
  title: string;
  state: State;
  spec: { id: string; title: string };
  labels: string[];
  tags: Record<string, string>;
  blocked_by: string[];
  blockers: BlockerView[];
  replacements: string[];
  created: string;
  updated: string;
  description: string;
  // branch and commit are the work's branch and head commit, as the last
  // finish naming them gave them. Work with a branch is Accepted, then lands.
  branch?: string;
  commit?: string;
  claims: ClaimView[];
  rounds: RoundView[];
  failure_reports: FailureReportView[];
  comments: CommentView[];
  moves: MoveView[];
  edits: EditView[];
  blocked: boolean;
  claimable: boolean;
  silent_since?: string;
  needs_me_reason?: NeedsMeReason;
  link?: LinkView;
}

// TICKET_FIELDS are TicketView's fields in the service's order, the order
// a TicketAcknowledgement's changed lists them in.
export const TICKET_FIELDS = [
  "id", "title", "state", "spec", "labels", "tags", "blocked_by", "blockers", "replacements",
  "created", "updated", "description", "branch", "commit", "claims", "rounds", "failure_reports",
  "comments", "moves", "edits", "blocked", "claimable", "silent_since", "needs_me_reason", "link",
] as const satisfies readonly (keyof TicketView)[];

// SPEC_FIELDS are SpecView's fields in the service's order, the order a
// SpecAcknowledgement's changed lists them in.
export const SPEC_FIELDS = [
  "id", "title", "labels", "tags", "created", "updated", "body", "edits", "link",
] as const satisfies readonly (keyof SpecView)[];

// SpecAcknowledgement is what a Spec write returns (ADR 0009): the names
// of the fields it changed, never their values. edit is set when the
// write recorded an edit.
export interface SpecAcknowledgement {
  id: string;
  updated: string;
  changed: (typeof SPEC_FIELDS)[number][];
  edit?: number;
}

// TicketAcknowledgement is what a Ticket write returns (ADR 0009): the
// names of the fields it changed, never their values, and the Ticket's
// State after it. edit is set when the write recorded an edit.
export interface TicketAcknowledgement {
  id: string;
  updated: string;
  changed: (typeof TICKET_FIELDS)[number][];
  edit?: number;
  state: State;
}

// The one error shape every door uses, plus the HTTP status.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly field?: string,
    readonly allowed_actions?: string[],
  ) {
    super(message);
    this.name = "ApiError";
  }
}

// A change notice from GET /api/events. It only names what changed; the
// UI refetches.
export type ChangeEvent =
  | { type: "ticket.changed"; project: string; id: string }
  | { type: "spec.changed"; project: string; id: string }
  | { type: "project.changed"; project: string }
  // options.changed is about no Project: the Options changed.
  | { type: "options.changed" };

// FocusClear is what Focus Mode shows when nothing needs the human.
export const FOCUS_CLEARS = ["all_clear", "activity_dashboard"] as const;
export type FocusClear = (typeof FOCUS_CLEARS)[number];

// Options are the human's global preferences, kept by the service
// (GET /api/options), each at its default until the human changes it.
export interface Options {
  focus_clear: FocusClear;
}

// DEFAULT_OPTIONS are the Options a fresh store reads, and what the UI
// uses while they load or when they can't be read.
export const DEFAULT_OPTIONS: Options = { focus_clear: "all_clear" };


// ---- Usage (Spec code-central-201) ----

// USAGE_DAYS are the ranges Usage reads, in local days; the first is the default.
export const USAGE_DAYS = [7, 30, 90] as const;
export type UsageDays = (typeof USAGE_DAYS)[number];

// UsageFilter is a Usage read: one Project, or all of them (Archived
// ones too) when project is left out, over the last days local days.
export interface UsageFilter {
  project?: string;
  days?: UsageDays;
}

// UsageOperation is one MCP operation's row in the Code Central calls
// table. Sizes are in bytes; averages are rounded to the nearest byte.
export interface UsageOperation {
  op: string;
  calls: number;
  request_bytes: number;
  average_request_bytes: number;
  response_bytes: number;
  average_response_bytes: number;
  errors: number;
}

// UsageKind is one kind of session's Context Size figures.
export interface UsageKind {
  kind: string;
  sessions: number;
  average_peak: number;
  largest_peak: number;
}

// UsageSession is one recent session, a Claim's main session or one of its subagents.
export interface UsageSession {
  project: string;
  ticket: string;
  claim: number;
  session_id: string;
  agent?: string;
  kind: string;
  started: string;
  peak: number;
  responses: number;
}

// UsageDay is one local day's token usage by kind of session.
export interface UsageDay {
  day: string;
  kinds: {
    kind: string;
    input_tokens: number;
    cache_write_tokens: number;
    cache_read_tokens: number;
    output_tokens: number;
  }[];
}

// UsageView is GET /api/usage. Kinds, sessions and days stay empty until
// the Factory posts token usage.
export interface UsageView {
  operations: UsageOperation[];
  kinds: UsageKind[];
  sessions: UsageSession[];
  days: UsageDay[];
}
