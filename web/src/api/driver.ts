// The Driver (Spec code-central-580): the one module that plays agents,
// integrations and the Factory, by making their real operations over a
// fetch, so every state a test or story reaches is the one Core derives.
// It implements the Driver below, the interface the contract suite (contract/)
// runs against, and the scenario
// builder (scenario.ts) is it plus helpers for whole flows.
//
// Its two adapters differ only in where the fetch goes and how what lies
// outside Core is set: worldDriver over a WASM Core world and its
// controls, and the serve half's (http.e2e.test.ts) over a real
// `code-central serve` with its fake gh and real connections. A write's
// ID is the one it returned, and a Question's number is Core's.
import type { Api } from "./api";
import type { CIRunView, CIWatcherReport, ExternalState, HeartbeatReport, ModelsReport, PullRequestReport, SessionEvent, TicketView, UsageResponse } from "./types";
import type { World } from "./core/core";
import { toError } from "./http";

// The Driver sets up the world the way agents, integrations and the Factory
// would, with their operations over HTTP. Tests use the IDs it returns,
// never literal IDs (Specs and Tickets share one counter).
export interface Driver {
  api: Api;
  createProject(key: string, displayName: string): Promise<void>;
  createTicket(key: string, title: string, state: "Draft" | "Ready"): Promise<string>;
  // postRound claims a Ready Ticket as an agent and posts a Round of
  // Questions, moving it to Needs Answer, and returns the Round's number.
  postRound(key: string, id: string, questions: { title: string; body: string; recommended?: string }[]): Promise<number>;
  // reportFailure claims a Ready Ticket as an agent (session
  // "contract-session") and posts a Failure Report, moving it to Failed.
  reportFailure(key: string, id: string, report: FailureReportBody): Promise<void>;
  // finish claims a Ready Ticket as an agent and finishes it with the
  // summary, moving it to In Review. on names the work's branch and head
  // commit (ticket 71), or a spec Stage's Spec recommendation.
  finish(
    key: string,
    id: string,
    summary: string,
    on?: { branch: string; commit: string } | { recommendation: NonNullable<TicketView["recommendation"]> },
  ): Promise<void>;
  // addBlocker makes id wait on blocker, as an agent's add_blocker.
  addBlocker(key: string, id: string, blocker: string): Promise<void>;
  // claim claims a Ready Ticket as an agent with a session ID, moving it
  // to In Progress (ticket 35).
  claim(key: string, id: string, sessionId: string): Promise<void>;
  // linkTicket creates a Draft Ticket linked to an external item as an
  // Integration would, and records its external state (ticket 42).
  linkTicket(key: string, title: string, link: { integration: string; id: string; url: string }, external: ExternalState): Promise<string>;
  // incomingIssue creates a Draft Ticket on spec linked to a GitHub issue,
  // as the Issue Watcher's create_linked_ticket does (code-central-287).
  incomingIssue(key: string, spec: string, title: string, link: { integration: string; id: string; url: string }): Promise<string>;
  // postSessionEvents posts a session's Session Events as a Factory would
  // (ticket 109): they go to the newest Claim with that session ID.
  postSessionEvents(sessionId: string, events: SessionEvent[]): Promise<void>;
  // mcpCall makes one agent MCP tool call to key's Project, as Claude Code
  // would, and returns the text the agent was given and, for a rejected
  // call, its error code (Spec code-central-201).
  mcpCall(key: string, op: "find_tickets" | "get_ticket", args: Record<string, unknown>): Promise<{ text: string; error?: string }>;
  // postUsage posts a session's model responses' token usage as a Factory
  // would (Spec code-central-201): they go to the newest Claim with that
  // session ID.
  postUsage(sessionId: string, responses: UsageResponse[]): Promise<void>;
  // dropTranscriptStreams drops every open Transcript stream, as a flaky
  // connection would; each reconnects by itself (ticket 110).
  dropTranscriptStreams(): void;
  // startConversation and sendChatMessage are the human's Chat actions;
  // sendChatMessage returns once the turn is queued (Spec code-central-633).
  startConversation(key: string): Promise<void>;
  sendChatMessage(key: string, text: string): Promise<void>;
  // takeChatTurn is a Factory's next_chat_work call, which takes the turn
  // queued, binding the Conversation to it; it returns the Conversation ID.
  takeChatTurn(key: string, factory: string): Promise<string>;
  // postChatEvents is the Factory holding the Conversation posting its
  // reply: text items, and the outcome on the last post.
  postChatEvents(key: string, factory: string, conversation: string, texts: string[], outcome?: "done" | "stopped" | "failed"): Promise<void>; // no outcome leaves the reply running
  // absolutePath is a path absolute where Core runs.
  readonly absolutePath: string;
  // reportPushRemote reports the Push remote's URL as a Factory does when
  // it starts (code-central-251).
  reportPushRemote(key: string, url: string): Promise<void>;
  // reportModels reports what the Factory's claude accepts, with its
  // flags, as a Factory does when it starts (code-central-311).
  reportModels(key: string, report: ModelsReport): Promise<void>;
  // reportHeartbeat sends a Factory's Heartbeat (code-central-324).
  reportHeartbeat(key: string, report: HeartbeatReport): Promise<void>;
  // logOutOfGH makes the gh the service finds logged out, so the CI check
  // refuses turning CI Tracking on (code-central-252).
  logOutOfGH(): Promise<void>;
  // letGHTriage makes the gh the service finds able to triage
  // repository's issues, so the Issues check passes (code-central-284).
  letGHTriage(repository: string): Promise<void>;
  // letGHWrite makes the gh the service finds able to write to
  // repository, so the Pull Request check passes on GitHub
  // (code-central-389).
  letGHWrite(repository: string): Promise<void>;
  // reportCIWatcherStatus reports the CI Watcher's status as the CI
  // Watcher does on each poll (code-central-253).
  reportCIWatcherStatus(key: string, status: CIWatcherReport): Promise<void>;
  // reportPushedCommit reports the commit a Factory pushed for an
  // Accepted Ticket, and reportCIRuns the CI Runs the CI Watcher saw
  // (code-central-256).
  // The branch it was pushed as is what the Pull Request Watcher opens a
  // pull request from; under Pull Request a push is recorded with CI
  // Tracking off too (code-central-388).
  reportPushedCommit(key: string, id: string, commit: string, branch?: string): Promise<void>;
  // reportPullRequest reports the pull request of the work pushed as
  // branch, as the Pull Request Watcher does; it is recorded on every
  // Ticket the work covers (code-central-388).
  reportPullRequest(key: string, report: PullRequestReport): Promise<void>;
  reportCIRuns(key: string, runs: Omit<CIRunView, "ignored">[]): Promise<void>;
  close(): Promise<void>;
}

export interface FailureReportBody {
  what_broke: string;
  why: string;
  what_to_fix: string;
  where_work_stands: string;
}


export const sampleReport: FailureReportBody = {
  what_broke: "Tests fail on CI\nlogin_test.go times out",
  why: "The fixture server never starts",
  what_to_fix: "Start the fixture in TestMain",
  where_work_stands: "Branch impl/7, two commits, tests red",
};


// A Connection is where a Driver's requests go: an adapter.
export interface Connection {
  // url is the service's origin, "" for a world.
  url: string;
  fetch: typeof fetch;
  // api is the UI's client of the same service.
  api: Api;
  // logOutOfGH, letGHTriage, letGHWrite and dropTranscriptStreams set
  // what lies outside Core, as the Driver's own do.
  logOutOfGH(): Promise<void>;
  letGHTriage(repository: string): Promise<void>;
  letGHWrite(repository: string): Promise<void>;
  dropTranscriptStreams(): void;
  // absolutePath is a path absolute where Core runs: a WASM Core judges
  // paths as Unix does, whatever the host.
  absolutePath: string;
  close(): Promise<void>;
}

export type Actor = "agent" | "human" | "integration";

// SESSION is the session an agent claims with unless told otherwise.
export const SESSION = "contract-session";

// worldDriver drives a WASM Core world.
export function worldDriver(w: World): HttpDriver {
  return new HttpDriver(worldConnection(w));
}

// worldConnection reaches a WASM Core world, and sets the gh checks with
// its controls: logged out as gh answers with no login.
export function worldConnection(w: World): Connection {
  const loggedOut = {
    problem: "logged_out",
    reason: "gh isn't logged in to GitHub: You are not logged into any GitHub hosts. To log in, run: gh auth login",
    fix: "Run `gh auth login` as the user the service runs as.",
  };
  return {
    url: "",
    fetch: w.fetch,
    api: w.api,
    logOutOfGH: async () => {
      w.setCICheck(loggedOut);
      w.setIssuesCheck(loggedOut);
      w.setPullRequestCheck(loggedOut);
    },
    letGHTriage: async () => w.setIssuesCheck(),
    letGHWrite: async () => w.setPullRequestCheck(),
    dropTranscriptStreams: () => w.dropStreams(),
    absolutePath: "/opt/tools/gh",
    close: async () => w.drop(),
  };
}

export class HttpDriver implements Driver {
  readonly api: Api;
  // specs are each Project's Spec that createTicket puts its Tickets
  // under: the one createProject made.
  protected readonly specs = new Map<string, string>();
  // claims are each Ticket's live Claim ID, kept as the agent holding it
  // keeps it for its next write.
  private readonly claims = new Map<string, string>();

  constructor(private readonly c: Connection) {
    this.api = c.api;
  }

  // call makes one request as actor and returns its JSON, nothing for a
  // 204, or throws its refusal as an ApiError.
  async call<T = any>(actor: Actor, method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
    const res = await this.c.fetch(this.c.url + path, {
      method,
      headers: { "Content-Type": "application/json", "X-Code-Central-Actor": actor },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await res.text();
    if (!res.ok) throw toError(res.status, text);
    return (text ? JSON.parse(text) : undefined) as T;
  }

  // agent, human and integration are one POST as that Actor.
  agent = <T = any>(path: string, body: unknown) => this.call<T>("agent", "POST", path, body);
  human = <T = any>(path: string, body: unknown) => this.call<T>("human", "POST", path, body);
  integration = <T = any>(path: string, body: unknown) => this.call<T>("integration", "POST", path, body);

  // projectPath and ticketPath are where a Project's and a Ticket's
  // operations go.
  projectPath = (key: string) => `/api/p/${key}`;
  ticketPath = (key: string, id: string) => `/api/p/${key}/tickets/${id.slice(id.lastIndexOf("-") + 1)}`;

  // claimFor is the Ticket's live Claim ID, claiming the Ticket as an
  // agent in session first when the Driver holds no Claim on it.
  async claimFor(key: string, id: string, session = SESSION): Promise<string> {
    return this.claims.get(id) ?? (await this.claimAs(key, id, session));
  }

  private async claimAs(key: string, id: string, session_id: string): Promise<string> {
    const { claim_id } = await this.agent<{ claim_id: string }>(`${this.projectPath(key)}/claim`, { ticket: id, session_id });
    this.claims.set(id, claim_id);
    return claim_id;
  }

  // ---- the contract's Driver ----

  // createProject makes the Project and a Spec for createTicket's Tickets.
  createProject: Driver["createProject"] = async (key, display_name) => {
    await this.agent("/api/projects/create_project", { key, display_name });
    this.specs.set(key, (await this.agent(`${this.projectPath(key)}/create_spec`, { title: "Contract", body: "" })).id);
  };

  createTicket: Driver["createTicket"] = async (key, title, state) =>
    (await this.agent(`${this.projectPath(key)}/create_ticket`, { spec: this.specs.get(key), title, state })).id;

  postRound: Driver["postRound"] = async (key, id, questions) =>
    (await this.agent(`${this.ticketPath(key, id)}/post_round`, { claim_id: await this.claimFor(key, id), questions })).round;

  reportFailure: Driver["reportFailure"] = async (key, id, report) => {
    await this.agent(`${this.ticketPath(key, id)}/report_failure`, { claim_id: await this.claimFor(key, id), ...report });
    this.claims.delete(id);
  };

  finish: Driver["finish"] = async (key, id, summary, on) => {
    await this.agent(`${this.ticketPath(key, id)}/finish`, { claim_id: await this.claimFor(key, id), summary, ...on });
    this.claims.delete(id);
  };

  addBlocker: Driver["addBlocker"] = async (key, id, blocker) => {
    await this.agent(`${this.ticketPath(key, id)}/add_blocker`, { blocker });
  };

  // ticket 35: always a new Claim, as a Ticket's next session makes.
  claim: Driver["claim"] = async (key, id, session_id) => {
    await this.claimAs(key, id, session_id);
  };

  // ticket 42
  linkTicket: Driver["linkTicket"] = async (key, title, link, external_state) => {
    const { id } = await this.integration(`${this.projectPath(key)}/integration/create_linked_ticket`, { spec: this.specs.get(key), title, link });
    await this.integration(`${this.projectPath(key)}/integration/set_external_state`, { target: id, external_state });
    return id;
  };

  // code-central-287
  incomingIssue: Driver["incomingIssue"] = async (key, spec, title, link) =>
    (await this.integration(`${this.projectPath(key)}/integration/create_linked_ticket`, { spec, title, link })).id;

  // ticket 109: as a Factory
  postSessionEvents: Driver["postSessionEvents"] = async (session_id, events) => {
    await this.agent("/api/session-events", { session_id, events });
  };

  // Spec code-central-201: one JSON-RPC tools/call to the Project's MCP
  // endpoint, which records it as Usage.
  mcpCall: Driver["mcpCall"] = async (key, op, args) => {
    const res = await this.c.fetch(`${this.c.url}/p/${key}/mcp`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json, text/event-stream", "X-Code-Central-Actor": "agent" },
      body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: op, arguments: args } }),
    });
    const { result } = await res.json();
    const text: string = result.content[0].text;
    return { text, error: result.isError ? JSON.parse(text).code : undefined };
  };

  // Spec code-central-201: as a Factory
  postUsage: Driver["postUsage"] = async (session_id, responses) => {
    await this.agent("/api/usage", { session_id, responses });
  };

  // code-central-251: as a Factory
  reportPushRemote: Driver["reportPushRemote"] = async (key, url) => {
    await this.agent(`${this.projectPath(key)}/report_push_remote`, { url });
  };

  // code-central-311: as a Factory
  reportModels: Driver["reportModels"] = async (key, report) => {
    await this.agent(`${this.projectPath(key)}/report_models`, report);
  };

  // code-central-324: as a Factory
  reportHeartbeat: Driver["reportHeartbeat"] = async (key, report) => {
    await this.agent(`${this.projectPath(key)}/report_heartbeat`, report);
  };

  // code-central-253: as the CI Watcher
  reportCIWatcherStatus: Driver["reportCIWatcherStatus"] = async (key, status) => {
    await this.agent(`${this.projectPath(key)}/report_ci_watcher_status`, status);
  };

  // code-central-256: as the Factory and the CI Watcher
  reportPushedCommit: Driver["reportPushedCommit"] = async (key, id, commit, branch) => {
    await this.agent(`${this.ticketPath(key, id)}/report_pushed_commit`, { commit, branch });
  };

  // code-central-388: as the Pull Request Watcher
  reportPullRequest: Driver["reportPullRequest"] = async (key, report) => {
    await this.agent(`${this.projectPath(key)}/report_pull_request`, report);
  };

  reportCIRuns: Driver["reportCIRuns"] = async (key, runs) => {
    await this.agent(`${this.projectPath(key)}/report_ci_runs`, { runs });
  };

  logOutOfGH: Driver["logOutOfGH"] = () => this.c.logOutOfGH();
  letGHTriage: Driver["letGHTriage"] = (repository) => this.c.letGHTriage(repository);
  letGHWrite: Driver["letGHWrite"] = (repository) => this.c.letGHWrite(repository);
  dropTranscriptStreams: Driver["dropTranscriptStreams"] = () => this.c.dropTranscriptStreams();

  // Spec code-central-633: the human's Chat actions, and a Factory's part
  startConversation: Driver["startConversation"] = async (key) => {
    await this.human(`${this.projectPath(key)}/start_conversation`, {});
  };
  sendChatMessage: Driver["sendChatMessage"] = async (key, text) => {
    await this.human(`${this.projectPath(key)}/send_chat_message`, { text });
  };
  takeChatTurn: Driver["takeChatTurn"] = async (key, factory) => {
    const work = await this.call<{ turn?: { conversation: string } }>("agent", "GET", `${this.projectPath(key)}/chat/next-work?factory=${factory}&host=laptop`);
    if (!work.turn) throw new Error("the Factory was handed no turn");
    return work.turn.conversation;
  };
  postChatEvents: Driver["postChatEvents"] = async (key, factory, conversation, texts, outcome) => {
    await this.agent(`${this.projectPath(key)}/chat/events`, {
      conversation,
      factory,
      host: "laptop",
      items: texts.map((text) => ({ time: "2026-11-08T10:00:00Z", kind: "text", text })),
      outcome: outcome ?? "",
    });
  };
  get absolutePath(): string {
    return this.c.absolutePath;
  }
  close: Driver["close"] = () => this.c.close();
}
