// The one API module: everything the UI reads, every human action and the
// live feed go through an Api. createHttpApi (http.ts) is the real one;
// FakeApi (fake.ts) is the in-memory fake the component and layout tests
// use. contract.ts runs one suite against both.
import type {
  ChangeEvent,
  CIOverviewFilter,
  CIOverview,
  FindSpecsFilter,
  FindTicketsFilter,
  HeldSessionEvent,
  CIRuleChange,
  CITrackingChange,
  Landing,
  LandingInput,
  LinkedRow,
  NeedsMeView,
  Options,
  ProjectRow,
  ProjectView,
  SpecAcknowledgement,
  SpecDetail,
  SpecQuestions,
  SpecRow,
  SubmitMapResult,
  SubmitRequirementResult,
  TicketAcknowledgement,
  TicketRow,
  TicketView,
  Transcript,
  UsageFilter,
  SessionUsageFilter,
  SessionUsageView,
  TargetView,
  UsageView, GitHubIssuesChange, Models
} from "./types";

// TicketActions maps each human action on one Ticket to its request body
// and result. Every one is `POST /api/p/{key}/tickets/{n}/{action}` with
// that body, so adding an action needs no new Api method: a panel's
// ticket declares its action by merging into this interface from its own
// file under ./actions/ (actions/move.ts is the example), which also adds
// the fake's behaviour with registerFakeAction (fakeActions.ts).
//
//   declare module "../api" {
//     interface TicketActions { answer: { body: { question: number; answer: string }; result: TicketView } }
//   }
export interface TicketActions {}
export type TicketAction = keyof TicketActions;

export interface EventHandlers {
  onEvent(e: ChangeEvent): void;
  // onOpen is called each time the feed is live: first with
  // reconnect=false, then reconnect=true after every reconnect, when
  // notices may have been missed and everything should be refetched.
  onOpen?(info: { reconnect: boolean }): void;
}

export interface TranscriptHandlers {
  // onEvent is called once for each Session Event, oldest first: the held
  // ones, then each new one. A reconnect resumes after the last one, so
  // none is repeated or missed.
  onEvent(e: HeldSessionEvent): void;
  // onTail is called before events that follow a gap: ones dropped (or
  // lost to a restart) before they could be sent, so the follower holds
  // only the tail of the session.
  onTail?(): void;
  // onOpen is called each time the stream is live: first with
  // reconnect=false, then reconnect=true after every reconnect.
  onOpen?(info: { reconnect: boolean }): void;
}

export interface Api {
  listProjects(): Promise<ProjectRow[]>;
  needsMe(): Promise<NeedsMeView>;
  // findTickets leaves out Done and Cancelled Tickets unless the filter names them.
  findTickets(project: string, filter?: FindTicketsFilter): Promise<TicketRow[]>;
  getTicket(project: string, id: string): Promise<TicketView>;
  getSpec(project: string, spec: string): Promise<SpecDetail>;
  // findSpecs leaves out finished Specs unless the filter asks for them.
  findSpecs(project: string, filter?: FindSpecsFilter): Promise<SpecRow[]>;
  // submitRequirement creates a Requirement from the human's text: its
  // Spec and three Stage Tickets, Grilling ready for a Factory. With map,
  // a finished Map, it writes the Spec from that Map: only the Spec and
  // Tickets Stages, Spec ready, and the Spec's map Tag names the Map. With
  // skipGrilling (Straight to Spec) it makes only those two Stages, from the
  // text alone, and no map Tag; it can't be given with map.
  submitRequirement(project: string, text: string, map?: string, skipGrilling?: boolean): Promise<SubmitRequirementResult>;
  // submitMap creates a Map from the human's text: its Spec, with the
  // Map's empty sections, and a Charting Ticket ready for a Factory.
  submitMap(project: string, text: string): Promise<SubmitMapResult>;
  // submitPrototype asks for a Prototype on any Spec from the human's
  // text: a Ready prototype Ticket titled by its first line.
  submitPrototype(project: string, spec: string, text: string): Promise<TicketAcknowledgement>;
  // submitSmallChange opens a Ready Ticket on the Project's Small Changes
  // Spec from the human's text, titled by its first line.
  submitSmallChange(project: string, text: string): Promise<TicketAcknowledgement>;
  // specQuestions rolls up a Spec's Questions: its Tickets' Rounds.
  specQuestions(project: string, spec: string): Promise<SpecQuestions>;
  act<A extends TicketAction>(
    project: string,
    id: string,
    action: A,
    body: TicketActions[A]["body"],
  ): Promise<TicketActions[A]["result"]>;
  // editSpec changes a Spec's title or body, sending only what changed
  // (edit_spec). The text it replaces is kept in the Spec's edits; a
  // change that changes nothing writes nothing, and an empty title is
  // refused as invalid. A Spec in any State takes it: the UI decides
  // where to offer it.
  editSpec(project: string, spec: string, change: { title?: string; body?: string }): Promise<SpecAcknowledgement>;
  // archiveProject puts away a Project whose Tickets are all finished;
  // an Archived Project refuses every write until unarchiveProject.
  archiveProject(project: string): Promise<ProjectView>;
  unarchiveProject(project: string): Promise<ProjectView>;
  // renameProject changes a Project's display name, never its key.
  renameProject(project: string, displayName: string): Promise<ProjectView>;
  // setLandingMethod sets how the Project's accepted work lands. The
  // remote is trimmed; under Push it can't be empty, and without Push an
  // empty one is stored as origin.
  setLandingMethod(project: string, landing: LandingInput): Promise<ProjectView>;
  // setCITracking sets the Project's CI settings. The repository is
  // trimmed, and needed as owner/name to turn CI Tracking on; Ignored
  // workflows are trimmed, with blanks and repeats dropped.
  setCITracking(project: string, ci: CITrackingChange): Promise<ProjectView>;
  // setGitHubIssues sets the Project's GitHub Issues settings. The
  // repository and label are trimmed; turning it on needs a repository as
  // owner/name and a passing Issues check, and the first time creates the
  // Issues Spec, titled like "GitHub Issues: acme/web", which follows a
  // changed repository unless the human retitled it.
  setGitHubIssues(project: string, settings: GitHubIssuesChange): Promise<ProjectView>;
  // setModels replaces the Project's Models rows whole: a kind left out
  // is set to Default. A model is trimmed and can't be blank or span
  // lines; an effort must be one of the valid levels.
  setModels(project: string, models: Models): Promise<ProjectView>;
  // setSubagentExploration turns on or off the implementation guide's
  // subagent line, from the next Claim and the next get_stage_guide.
  setSubagentExploration(project: string, on: boolean): Promise<ProjectView>;
  // setModelAssignment turns on or off the tickets guide's model and effort
  // assignment (human-only), off to begin with.
  setModelAssignment(project: string, on: boolean): Promise<ProjectView>;
  // addCIRule adds a CI Rule and creates its Spec, titled like "CI: ci.yml
  // on main"; editCIRule changes the rule on Spec spec, and
  // removeCIRule removes it, leaving its Spec as the record. Workflow and
  // branch are trimmed; two rules can't watch the same workflow and branch.
  addCIRule(project: string, rule: CIRuleChange): Promise<ProjectView>;
  editCIRule(project: string, spec: string, rule: CIRuleChange): Promise<ProjectView>;
  removeCIRule(project: string, spec: string): Promise<ProjectView>;
  // unlink detaches a Spec's or Ticket's Link (ticket 42). It isn't a
  // TicketAction: its target, a Spec or a Ticket, goes in the body.
  unlink(project: string, target: string): Promise<LinkedRow>;
  // setTags merges tags into a Spec's or Ticket's Tags, overwriting only
  // the keys it names; removeTags removes the keys named.
  setTags(project: string, target: string, tags: Record<string, string>): Promise<TargetView>;
  removeTags(project: string, target: string, keys: string[]): Promise<TargetView>;
  // options reads the human's Options; setOptions changes those given and
  // answers with them all as they now stand. An unknown Option or a value
  // outside its set is refused as invalid, naming it, and nothing changes.
  options(): Promise<Options>;
  setOptions(change: Partial<Options>): Promise<Options>;
  // usage reads Usage: the Code Central calls agents made in one Project,
  // or all of them, over the last 7 (the default), 30 or 90 local days.
  // An unknown Project is not_found.
  usage(filter?: UsageFilter): Promise<UsageView>;
  // ciOverview reads the CI view (code-central-258): one Project's CI Runs, or
  // every active Project's with CI Tracking on, and their CI Watchers.
  // An unknown Project is not_found.
  ciOverview(filter?: CIOverviewFilter): Promise<CIOverview>;
  // sessionUsage reads one session's responses, oldest first, each with
  // its Context Size, for its line chart. An unknown Project is not_found;
  // an unknown session has none.
  sessionUsage(filter: SessionUsageFilter): Promise<SessionUsageView>;
  // subscribe opens the live feed and returns a function that closes it.
  subscribe(handlers: EventHandlers): () => void;
  // transcript reads Claim number claim's Transcript (ticket 110).
  transcript(project: string, id: string, claim: number): Promise<Transcript>;
  // subscribeTranscript follows Claim number claim's Transcript, even
  // after the Claim ends, and returns a function that stops it.
  subscribeTranscript(project: string, id: string, claim: number, handlers: TranscriptHandlers): () => void;
}

// projectOf is a display ID's Project Key: everything before the last "-".
export function projectOf(id: string): string {
  const i = id.lastIndexOf("-");
  return i < 0 ? "" : id.slice(0, i);
}

// numberOf is a display ID's number: everything after the last "-".
export function numberOf(id: string): string {
  return id.slice(id.lastIndexOf("-") + 1);
}
