// The wire types: the JSON the HTTP API returns. Every response shape is
// generated from the Core's Go view types into wire.gen.ts by `make
// wiretypes` (ADR 0042) and re-exported here, the UI's one place to
// import them from. This file keeps only what isn't a response shape:
// request bodies and filters, types the UI derives from the generated
// ones, and UI-only constants.

import {
  FOCUS_CLEARS,
  FOCUS_POPUPS,
  LIMITS_POPUPS,
  type Acknowledgement,
  type BaseDivergedView,
  type BaseMissingView,
  type ChatMessageView,
  type CIItemView,
  type CIProvider,
  type CIRuleView,
  type CIUnavailableView,
  type CIView,
  type ConversationView,
  type FactoryStatusView,
  type GitHubIssuesView,
  type HeldSessionEvent,
  type LandingView,
  type LandPerSpecView,
  type ModelRowView,
  type ModelsView,
  type OptionsView,
  type PlanLimits,
  type ProjectView,
  type PullRequestState,
  type ReportedModelsView,
  type SPEC_FIELDS,
  type SpecRow,
  type State,
  type TICKET_FIELDS,
  type TranscriptView,
  type WatcherState,
  type WatcherView,
} from "./wire.gen";

export * from "./wire.gen";

// A finished Ticket is Done or Cancelled.
export const FINISHED: readonly State[] = ["Done", "Cancelled"];

// ---- The UI's names for generated views whose Go names differ ----

export type Landing = LandingView;
export type CISettings = CIView;
export type CIRule = CIRuleView;
export type GitHubIssuesSettings = GitHubIssuesView;
export type CIWatcherStatus = WatcherView;
export type IssueWatcherStatus = WatcherView;
export type PullRequestWatcherStatus = WatcherView;
export type AutoAccept = ProjectView["auto_accept"];
export type ModelRow = ModelRowView;
export type Models = ModelsView;
export type ReportedModels = ReportedModelsView;
export type FactoryStatus = FactoryStatusView;
export type BaseDiverged = BaseDivergedView;
export type BaseMissing = BaseMissingView;
export type CIItem = CIItemView;
export type CIUnavailable = CIUnavailableView;
export type LandPerSpecStatus = LandPerSpecView;
export type StateSummary = SpecRow["state_summary"];
export type Transcript = TranscriptView;
export type Options = OptionsView;
export type ChatMessage = ChatMessageView;
export type Conversation = ConversationView;
// A reply's item: a Session Event as a Conversation holds it.
export type ChatItem = HeldSessionEvent;

// ---- Landing ----

// What setLandingMethod takes: the Code Host and repository are derived, and
// leaving land_per_spec, pat_env or base out keeps the stored value.
export type LandingInput = Omit<Landing, "land_per_spec" | "code_host" | "repository" | "pat_env" | "base" | "watcher" | "open_pull_requests"> & {
  land_per_spec?: boolean;
  pat_env?: string;
  base?: string;
};

// A Watcher's report of its status: the service adds since.
export type CIWatcherReport = Omit<WatcherView, "since"> & { state: Exclude<WatcherState, "not_reported"> };
export type IssueWatcherReport = CIWatcherReport;
export type PullRequestWatcherReport = CIWatcherReport;

// What the Pull Request Watcher reports of the pull request of the work
// pushed as branch; merge_commit comes with the merged state alone.
export interface PullRequestReport {
  branch: string;
  number: number;
  link: string;
  state: PullRequestState;
  merge_commit?: string;
}

// ---- CI ----

// The CI settings the human sets with setCITracking.
export interface CITrackingChange {
  tracking: boolean;
  provider: CIProvider;
  // repository is the GitHub repository as owner/name; CI Tracking on needs one.
  repository: string;
  // project_url is the Azure DevOps project as its URL, such as
  // https://dev.azure.com/acme/web; Azure Pipelines on needs one. Each
  // CI Provider keeps its own fields, so switching loses nothing.
  project_url?: string;
  // pat_env names the environment variable holding the Azure DevOps PAT in
  // the service's environment, ADO_PAT when left empty. The PAT itself is
  // never kept.
  pat_env?: string;
  // ignored_workflows are workflow names: workflow file names, such as
  // ci.yml, or pipeline names.
  ignored_workflows: string[];
}

// A CI Rule as the human adds or edits it: when workflow fails on branch,
// open a Ticket to fix it, in state.
export interface CIRuleChange {
  // workflow is a workflow file name, such as ci.yml, or a pipeline's
  // name, or "any".
  workflow: string;
  // branch is an exact branch name, with no globs.
  branch: string;
  // state is the State the rule's Tickets open in.
  state: "Draft" | "Ready";
}

// CI_VIEW_DAYS is how far back the CI view lists finished runs.
export const CI_VIEW_DAYS = 7;

// CIOverviewFilter is a CI view read: one Project, or every active Project with
// CI Tracking on when project is left out. running keeps only the runs that
// count as running towards a Ticket's CI result (code-central-276).
export interface CIOverviewFilter {
  project?: string;
  running?: boolean;
}

// ---- GitHub Issues ----

// The GitHub Issues settings the human sets with setGitHubIssues (Spec
// code-central-279).
export interface GitHubIssuesChange {
  on: boolean;
  // repository is the GitHub repository as owner/name; on needs one.
  repository: string;
  // label is the filter label: only issues carrying it come in; "" lets
  // every issue in.
  label: string;
  new_issues_need_me: boolean;
}

// ---- Models and Factories ----

// What set_models takes: the rows whole, with the Chat row left empty
// when it is left out.
export type ModelsChange = Omit<Models, "chat"> & { chat?: ModelRow };

// EFFORT_LEVELS are the valid effort levels until a Factory reports its
// claude's.
export const EFFORT_LEVELS = ["low", "medium", "high", "xhigh", "max"];

// ModelsReport is a Factory's startup report of what its claude accepts,
// with its own --model and --effort flags, "" when unset.
export interface ModelsReport {
  effort_levels: string[];
  aliases: string[];
  model_flag: string;
  effort_flag: string;
}

// What a Factory sends as a Heartbeat.
export interface HeartbeatReport {
  factory: string;
  host: string;
  state: "online" | "draining" | "stopping";
  note?: string;
  interval_seconds: number;
  // chat defaults to false.
  chat?: boolean;
  // plan_limits are the Plan Limits the Factory last read from its Turns;
  // left out, they are none.
  plan_limits?: PlanLimits;
}

// ---- Specs and Tickets ----

// GUIDED_TAG marks a Map as guided when its value is GUIDED_ON; a missing
// Tag or any other value means off.
export const GUIDED_TAG = "guided";
export const GUIDED_ON = "on";

// MAP_LABEL marks a Spec as a Map.
export const MAP_LABEL = "wayfinder:map";

// LONG_LIVED_LABEL marks a Spec as a Long-lived Spec, whose Tickets land one
// by one under Land per Spec.
export const LONG_LIVED_LABEL = "long-lived";

export interface FindSpecsFilter {
  // requirements keeps only Requirements.
  requirements?: boolean;
  // labels keeps only Specs with every one of these Labels.
  labels?: string[];
  // finished includes finished Specs, which are otherwise left out.
  finished?: boolean;
}

export interface FindTicketsFilter {
  state?: State[];
}

// SpecAcknowledgement is what a Spec write returns (ADR 0009): the names
// of the fields it changed, never their values. A Spec has no State.
export interface SpecAcknowledgement extends Omit<Acknowledgement, "state"> {
  changed: (typeof SPEC_FIELDS)[number][];
}

// TicketAcknowledgement is what a Ticket write returns (ADR 0009): the
// names of the fields it changed, never their values, and the Ticket's
// State after it.
export interface TicketAcknowledgement extends Acknowledgement {
  changed: (typeof TICKET_FIELDS)[number][];
  state: State;
}

// submit_requirement's result (ADR 0009): the acknowledgements of the
// new Spec and its Stage Tickets, in order: three, or two from a Map.
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

// ---- Errors and change notices ----

// The one error shape every door uses, plus the HTTP status.
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly field?: string,
    readonly allowed_actions?: string[],
    // fix says how to put right what was refused, when the message alone
    // doesn't: a failed CI check's fix, for one.
    readonly fix?: string,
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
  // ci.changed is a Project's CI Runs changing.
  | { type: "ci.changed"; project: string }
  // chat.changed is a Project's Conversation changing: a new one, a reply
  // starting or ending, or its availability changing.
  | { type: "chat.changed"; project: string }
  // options.changed is about no Project: the Options changed.
  | { type: "options.changed" };

// ---- Options ----

// DEFAULT_OPTIONS are the Options a fresh store reads.
export const DEFAULT_OPTIONS: Options = { focus_clear: FOCUS_CLEARS[0], focus_popup: FOCUS_POPUPS[0], limits_popup: LIMITS_POPUPS[0], gh_path: "" };

// ---- Usage (Spec code-central-201) ----

// USAGE_DAYS are the ranges Usage reads, in local days; the first is the default.
export const USAGE_DAYS = [7, 30, 90] as const;
export type UsageDays = (typeof USAGE_DAYS)[number];
// USAGE_KEPT_DAYS is how many days before today Usage keeps records: a
// session's read covers today and those days.
export const USAGE_KEPT_DAYS = 90;

// UsageFilter is a Usage read: one Project, or all of them (Archived
// ones too) when project is left out, over the last days local days.
export interface UsageFilter {
  project?: string;
  days?: UsageDays;
}

// UsageResponse is one model response's token usage as a Factory posts
// it to POST /api/usage; agent, agent_type and description are left out
// for the main session, agent_type and description for a Subagent whose
// Agent call gave neither, and model when the Factory has none.
export interface UsageResponse {
  time: string;
  agent?: string;
  agent_type?: string;
  description?: string;
  kind: string;
  model?: string;
  input_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  output_tokens: number;
}

// SessionUsageFilter is one session's read: its Project, its session ID,
// and its subagent, left out for the main session.
export interface SessionUsageFilter {
  project: string;
  session: string;
  agent?: string;
}

// sessionKey names one session, the same for every filter that reads it.
export function sessionKey({ project, session, agent }: SessionUsageFilter): string {
  return `${project}\u0000${session}\u0000${agent ?? ""}`;
}
