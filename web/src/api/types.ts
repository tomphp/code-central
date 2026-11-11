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
  | "ci_failed"
  | "ci_cancelled"
  | "to_merge"
  | "ci_unavailable"
  | "accepted_not_landed"
  | "blocked_by_cancelled"
  | "new_issue"
  | "closed_externally";

// A triage action the human may take on an Incoming Issue (Spec
// code-central-279), each an operation's name.
export type TriageAction = "keep_as_draft" | "to_requirement" | "wont_fix";

// TRIAGE_ACTIONS are the triage actions, in the order they are offered.
export const TRIAGE_ACTIONS: TriageAction[] = ["keep_as_draft", "to_requirement", "wont_fix"];

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

// A CI Provider: the CI system CI Tracking watches.
export type CIProvider = "github_actions" | "azure_pipelines";

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
  // unblock_on_push lets a Ticket the CI gate holds stop blocking while
  // its pushed commit's CI runs or has passed. It may be on while CI
  // Tracking is off.
  unblock_on_push: boolean;
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

// A CI Rule, named by the ID of its own Spec, on which its Tickets open.
export interface CIRule extends CIRuleChange {
  spec: string;
  created: string;
}

// Why the CI Watcher can't watch its repository, each with its own fix.
export type CIProblem =
  | "gh_missing"
  | "logged_out"
  | "repository_unreadable"
  | "pat_missing"
  | "pat_rejected"
  | "project_unreadable"
  | "pat_scope"
  | "check_failed"
  | "crashed";

// The CI Watcher's status as it last reported it (code-central-253):
// not_reported until it has since the service started or CI Tracking was
// turned on, then watching, or an error with its message and fix.
export interface CIWatcherStatus {
  state: "not_reported" | "watching" | "error";
  problem?: CIProblem;
  message?: string;
  fix?: string;
  // rate_limited_until is when GitHub lets a rate-limited CI Watcher call
  // it again; it is still watching meanwhile, not in error.
  rate_limited_until?: string;
  // since is when it went into this state.
  since?: string;
}

// A CI Watcher's report of its status: the service adds since.
export type CIWatcherReport = Omit<CIWatcherStatus, "since"> & { state: "watching" | "error" };

// A Project's CI settings, with the URL of the Push remote the Factory
// last reported ("" until it has), the CI Rules, in the order added, and
// the CI Watcher's status.
export interface CISettings extends CITrackingChange {
  project_url: string;
  pat_env: string;
  push_remote_url: string;
  rules: CIRule[];
  watcher: CIWatcherStatus;
}

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

// Why the Issue Watcher can't work its repository's issues, each with its
// own fix: the CI Watcher's problems, and issues turned off or a login
// that can't triage them.
export type IssuesProblem = CIProblem | "issues_disabled" | "no_triage";

// The Issue Watcher's status as it last reported it (code-central-285),
// as the CI Watcher's is.
export interface IssueWatcherStatus extends Omit<CIWatcherStatus, "problem"> {
  problem?: IssuesProblem;
}

// An Issue Watcher's report of its status: the service adds since.
export type IssueWatcherReport = Omit<IssueWatcherStatus, "since"> & { state: "watching" | "error" };

// A Project's GitHub Issues settings, with its Issues Spec's ID once
// GitHub Issues has first been turned on, and the Issue Watcher's status.
export interface GitHubIssuesSettings extends GitHubIssuesChange {
  spec?: string;
  watcher: IssueWatcherStatus;
}

// A Models row (Spec code-central-301): the model and effort it sets,
// each "" for Default, which leaves it to the tier below.
export interface ModelRow {
  model: string;
  effort: string;
}

// The Models row for one kind of work, a Stage Guide name.
export interface KindModels extends ModelRow {
  kind: string;
}

// A Project's Models: the Project default row, then a row for every kind
// of work, in the guides' order.
export interface Models {
  default: ModelRow;
  kinds: KindModels[];
}

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

// A Project's Reported Models: what its Factory last reported, plus the
// model IDs its sessions' usage has shown. effort_levels are the valid
// levels: the reported ones, or EFFORT_LEVELS until a report gives some.
// reported is when the Factory last reported, missing before any report.
export interface ReportedModels extends ModelsReport {
  model_ids: string[];
  reported?: string;
}

// Which tier set a Model Choice value: the Ticket's Tag, the Project's
// Models row for its kind of work, the Project default row, or none of
// them, which leaves it to the Factory's flag or Claude Code's default.
export type ChoiceSource = "ticket_tag" | "project_kind" | "project_default" | "none";

// A Model Choice value with its source; value is missing when the
// source is none. kind names the kind of work whose Models row set it,
// only when the source is project_kind.
export interface ChoiceValue {
  value?: string;
  source: ChoiceSource;
  kind?: string;
}

// The model and effort a Ticket's next turn runs with, as far as Code
// Central sets them, each worked out on its own.
export interface ModelChoice {
  model: ChoiceValue;
  effort: ChoiceValue;
}

// A Ticket's CI result (code-central-256), from the runs of its latest
// pushed commit that aren't ignored: running while any is unfinished or
// none is reported, failed if any concluded failure, cancelled or timed
// out, passed otherwise.
export type CIResult = "running" | "passed" | "failed" | "cancelled";

// Where a CI Run stands, and how a finished run or job ended.
export type CIRunStatus = "queued" | "in_progress" | "completed";
export type CIConclusion = "success" | "failure" | "cancelled" | "timed_out" | "skipped" | "neutral" | "partially_succeeded";

// A commit the Factory pushed for a Ticket while CI Tracking was on.
export interface PushedCommitView {
  commit: string;
  pushed: string;
}

// One job of a CI Run; its conclusion is left out until it ends.
export interface CIJobView {
  name: string;
  conclusion?: CIConclusion;
  url: string;
}

// One attempt of one workflow run, as the CI Watcher reported it, tracked
// against a Ticket or a CI Rule's Spec. An ignored run is of an Ignored
// workflow, and never counts towards a CI result.
export interface CIRunView {
  id: number;
  attempt: number;
  // workflow is the workflow file's base name, such as ci.yml; name is
  // the name it shows.
  workflow: string;
  name: string;
  branch: string;
  commit: string;
  status: CIRunStatus;
  conclusion?: CIConclusion;
  started?: string;
  finished?: string;
  url: string;
  jobs: CIJobView[];
  ticket?: string;
  rule?: string;
  ignored: boolean;
}

// ---- The CI view (code-central-258) ----

// CI_VIEW_DAYS is how far back the CI view lists finished runs.
export const CI_VIEW_DAYS = 7;

// CIOverviewFilter is a CI view read: one Project, or every active Project with
// CI Tracking on when project is left out. running keeps only the runs that
// count as running towards a Ticket's CI result (code-central-276).
export interface CIOverviewFilter {
  project?: string;
  running?: boolean;
}

// A Project's CI Tracking and its CI Watcher's status, heading the CI view.
export interface CIProjectStatus {
  project: string;
  tracking: boolean;
  provider: CIProvider;
  repository: string;
  project_url: string;
  watcher: CIWatcherStatus;
}

// One CI Run in the CI view, with its Project; a re-run lists its latest
// attempt only.
export interface CIRunRow extends CIRunView {
  project: string;
}

// The CI view: the CI Watchers' statuses, then the runs, in-progress
// ones first, newest started first, then those finished in the last
// CI_VIEW_DAYS days, newest finished first.
export interface CIOverview {
  projects: CIProjectStatus[];
  runs: CIRunRow[];
}

export type FactoryStatusName = "online" | "draining" | "offline";

// A Factory as the Project view lists it, from its last Heartbeat.
export interface FactoryView {
  name: string;
  host: string;
  status: FactoryStatusName;
  // note is "" when none; a stopped Factory keeps its last one.
  note: string;
  last_heartbeat: string;
  interval_seconds: number;
}

// A Project's Factory Status, worked out by Code Central: the UI only
// renders it. status is the combined one (offline with no Factories).
export interface FactoryStatus {
  status: FactoryStatusName;
  factories: FactoryView[];
}

// What a Factory sends as a Heartbeat.
export interface HeartbeatReport {
  factory: string;
  host: string;
  state: "online" | "draining" | "stopping";
  note?: string;
  interval_seconds: number;
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
  // ci is always filled in: CI Tracking off by default.
  ci: CISettings;
  // github_issues is always filled in: GitHub Issues off by default.
  github_issues: GitHubIssuesSettings;
  // models is always filled in: every row at Default to begin with.
  models: Models;
  // reported_models is always filled in: no aliases or model IDs, and
  // the seeded effort levels, until a Factory reports.
  reported_models: ReportedModels;
  // factory_status is always filled in: offline, with no Factories,
  // until one sends a Heartbeat.
  factory_status: FactoryStatus;
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
  // outbound marks a Link that doesn't own the item's definition: Code
  // Central owns it and only writes to the external item (To Requirement).
  outbound: boolean;
}

// set_tags' and remove_tags' result: the Spec or Ticket with its Labels
// and Tags as they now stand.
export interface TargetView {
  id: string;
  kind: "spec" | "ticket";
  labels: string[];
  tags: Record<string, string>;
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
  // ci_result is the Blocker's CI result while the CI gate holds it,
  // whether or not Unblock on push is on.
  ci_result?: CIResult;
  // cleared is whether the Blocker no longer blocks though it isn't Done,
  // which only Unblock on push does.
  cleared: boolean;
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
  // ci_failure is what failed on the pushed commit (ci_failed).
  ci_failure?: CIFailureSummary;
  // ci_unavailable says why no word of its CI has come (ci_unavailable).
  ci_unavailable?: CIUnavailable;
  blocker?: BlockerView;
  link?: LinkView;
  // Set when the Ticket's Spec is a Requirement.
  requirement?: NeedsMeRequirement;
  // spec is a Map Ticket's or Prototype's Spec, as its context, set when
  // that Spec isn't a Requirement.
  spec?: NeedsMeSpec;
  // prototype is set on a Ticket labelled prototype, for its badge.
  prototype?: boolean;
  // ci_result is the Ticket's CI result, whatever the reason, for its
  // badge; left out for a Ticket never pushed while CI Tracking was on.
  ci_result?: CIResult;
}

// What failed on a CI failed Ticket's pushed commit: the commit and its
// failed runs' workflow file names.
export interface CIFailureSummary {
  commit: string;
  workflows: string[];
}

// A failure option the human may choose for a CI failed Ticket, named for
// its action (code-central-261, code-central-264, code-central-265).
export type CIFailureOption = "rerun_ci" | "land_anyway" | "send_back" | "new_fix_ticket";

// Why a Ticket is CI unavailable: the CI Watcher isn't Watching (with
// its error's message, if it has one), or no CI Runs have been reported
// for its pushed commit.
export interface CIUnavailable {
  cause: "ci_watcher_not_watching" | "no_ci_runs";
  commit: string;
  pushed: string;
  message?: string;
}

// The Spec a Needs Me row's Map Ticket or Prototype is on; map is set
// when it is a Map.
export interface NeedsMeSpec {
  spec: string;
  title: string;
  map?: boolean;
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
  // map is the Map the Requirement was made from (Write the Spec), which
  // then has no grilling Stage.
  map?: string;
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
  // guided is whether its guided Tag is on: the Factory then starts its
  // next decision whenever the Map is idle.
  guided: boolean;
  // can_write_spec is whether it has no unfinished Tickets.
  can_write_spec: boolean;
  // requirements are the Specs Write the Spec made from it: those whose
  // map Tag names it.
  requirements: string[];
}

// GUIDED_TAG marks a Map as guided when its value is GUIDED_ON; a missing
// Tag or any other value means off.
export const GUIDED_TAG = "guided";
export const GUIDED_ON = "on";

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
  // blocked_by is the Ticket's Blockers on a Map, the Map view's edges;
  // left out on any other Spec.
  blocked_by?: string[];
  // latest_event is the latest Session Event of the Ticket's live Claim;
  // left out when there is none.
  latest_event?: LatestEvent;
  // ci_result is the Ticket's CI result, for its badge; left out for a
  // Ticket never pushed while CI Tracking was on.
  ci_result?: CIResult;
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
  // What the Claim's latest turn ran with; left out for Claude Code's
  // default.
  model?: string;
  effort?: string;
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
  // pushed_commit is the latest commit the Factory pushed while CI
  // Tracking was on, and ci_result its CI result; ci_runs are every CI Run
  // tracked against the Ticket, the latest pushed commit's first. All are
  // left out while it has none (code-central-256).
  pushed_commit?: PushedCommitView;
  ci_result?: CIResult;
  ci_runs?: CIRunView[];
  // land_anyway says the human accepted the latest pushed commit's failed
  // CI result, so the Factory lands it; ci_failure_options are the
  // failure options the human may choose while the Ticket is CI failed,
  // left out otherwise (code-central-261).
  land_anyway?: boolean;
  ci_failure_options?: CIFailureOption[];
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
  // awaiting_triage is set on an Incoming Issue that came in while New
  // issues need me was on and that the human hasn't triaged yet;
  // triage_actions are the triage actions offered on an Incoming Issue,
  // left out on any other Ticket (code-central-287).
  awaiting_triage?: boolean;
  triage_actions?: TriageAction[];
  // model_choice is the model and effort the Ticket's next turn runs
  // with, each with its source; only the UI's read carries it (Spec
  // code-central-301).
  model_choice?: ModelChoice;
}

// TICKET_FIELDS are TicketView's fields in the service's order, the order
// a TicketAcknowledgement's changed lists them in.
export const TICKET_FIELDS = [
  "id", "title", "state", "spec", "labels", "tags", "blocked_by", "blockers", "replacements",
  "created", "updated", "description", "branch", "commit", "pushed_commit", "ci_result", "ci_runs", "land_anyway", "ci_failure_options", "claims", "rounds", "failure_reports",
  "comments", "moves", "edits", "blocked", "claimable", "silent_since", "needs_me_reason", "link",
  "awaiting_triage", "triage_actions",
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
  // options.changed is about no Project: the Options changed.
  | { type: "options.changed" };

// FocusClear is what Focus Mode shows when nothing needs the human.
// The default first.
export const FOCUS_CLEARS = ["activity_dashboard", "all_clear"] as const;
export type FocusClear = (typeof FOCUS_CLEARS)[number];

// Options are the human's global preferences, kept by the service
// (GET /api/options), each at its default until the human changes it.
export interface Options {
  focus_clear: FocusClear;
  // gh_path is where gh, the GitHub CLI, is: an absolute path, or "" to
  // find it on the service's PATH, then in the usual install places.
  gh_path: string;
}

// DEFAULT_OPTIONS are the Options a fresh store reads.
export const DEFAULT_OPTIONS: Options = { focus_clear: FOCUS_CLEARS[0], gh_path: "" };


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

// UsageKind is one kind of session's Context Size figures: its sessions,
// and the average (to the nearest token) and largest of their peaks.
export interface UsageKind {
  kind: string;
  sessions: number;
  average_peak: number;
  largest_peak: number;
}

// UsageSession is one recent session, a Claim's main session or one of its
// subagents (agent), with the Claim and kind of its latest response; peak
// is its largest Context Size, started its first response's time, and
// models each distinct model its responses name, in order of first use
// (empty when none does).
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
  models: string[];
}

// UsageDay is one local day's token usage (day is YYYY-MM-DD in the
// service's time zone): the kinds of session with any usage that day, in
// the kinds' order; none on a day with no usage.
export interface UsageDay {
  day: string;
  kinds: UsageDayTokens[];
}

// UsageDayTokens is one kind of session's tokens on one day, by token type.
export interface UsageDayTokens {
  kind: string;
  input_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  output_tokens: number;
}

// UsageResponse is one model response's token usage as a Factory posts
// it to POST /api/usage; agent is left out for the main session, and
// model when the Factory has none.
export interface UsageResponse {
  time: string;
  agent?: string;
  kind: string;
  model?: string;
  input_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  output_tokens: number;
}

// UsageView is GET /api/usage: kinds, sessions and days come from the
// token usage a Factory posts, sessions newest first and at most 50, and
// days a row for every local day in the range, oldest first.
export interface UsageView {
  operations: UsageOperation[];
  kinds: UsageKind[];
  sessions: UsageSession[];
  days: UsageDay[];
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

// SessionUsageResponse is one of a session's model responses: when it
// came, its Context Size (input, cache write and cache read) and its four
// token counts.
export interface SessionUsageResponse {
  time: string;
  context_size: number;
  input_tokens: number;
  cache_write_tokens: number;
  cache_read_tokens: number;
  output_tokens: number;
}

// SessionUsageView is GET /api/usage/session: one session's responses,
// oldest first, over every day Usage keeps.
export interface SessionUsageView {
  responses: SessionUsageResponse[];
}
