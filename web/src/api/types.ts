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
  | "accepted_not_landed"
  | "blocked_by_cancelled"
  | "closed_externally";

// A Project's mode: where init puts its files.
export type Mode = "committed" | "local";

export interface ProjectView {
  key: string;
  display_name: string;
  created: string;
  mode: Mode;
  archived: boolean;
  // archived_at is when the human archived the Project, while it is.
  archived_at?: string;
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
  | { type: "project.changed"; project: string };
