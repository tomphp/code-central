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
  "Done",
  "Cancelled",
] as const;
export type State = (typeof STATES)[number];

export type NeedsMeReason =
  | "needs_answer"
  | "answered"
  | "silent_claim"
  | "failed"
  | "in_review"
  | "blocked_by_cancelled"
  | "closed_externally";

export interface ProjectRow {
  key: string;
  display_name: string;
  created: string;
  mode: string;
  archived: boolean;
  needs_me: number;
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

export interface ClaimView {
  number: number;
  session_id?: string;
  started: string;
  last_activity: string;
  ended?: string;
  ended_by?: string;
}

export interface FailureReportView {
  number: number;
  claim: number;
  session_id?: string;
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
