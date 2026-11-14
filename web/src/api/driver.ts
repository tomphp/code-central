// The Driver (Spec code-central-580): the one module that plays agents,
// integrations and the Factory, by making their real operations over a
// fetch, so every state a test or story reaches is the one Core derives.
// It implements the contract's Driver (contract.ts), and the scenario
// builder (scenario.ts) is it plus helpers for whole flows.
//
// Its two adapters differ only in where the fetch goes and how what lies
// outside Core is set: worldDriver over a WASM Core world and its
// controls, and the serve half's (http.e2e.test.ts) over a real
// `code-central serve` with its fake gh and real connections. A write's
// ID is the one it returned, and a Question's number is Core's.
import type { Api } from "./api";
import type { Driver } from "./contract";
import type { World } from "./core/core";
import { toError } from "./http";

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
    close: async () => w.drop(),
  };
}

export class HttpDriver implements Driver {
  readonly api: Api;
  // specs are the Spec createProject made in each Project, which
  // createTicket puts its Tickets under.
  private readonly specs = new Map<string, string>();

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

  // claimed claims a Ready Ticket as an agent and returns its Claim ID.
  async claimed(key: string, id: string, session_id = SESSION): Promise<string> {
    return (await this.agent<{ claim_id: string }>(`${this.projectPath(key)}/claim`, { ticket: id, session_id })).claim_id;
  }

  // ---- the contract's Driver ----

  // createProject makes the Project and a Spec for createTicket's Tickets.
  createProject: Driver["createProject"] = async (key, display_name) => {
    await this.agent("/api/projects/create_project", { key, display_name });
    this.specs.set(key, (await this.agent(`${this.projectPath(key)}/create_spec`, { title: "Contract", body: "" })).id);
  };

  createTicket: Driver["createTicket"] = async (key, title, state) =>
    (await this.agent(`${this.projectPath(key)}/create_ticket`, { spec: this.specs.get(key), title, state })).id;

  postRound: Driver["postRound"] = async (key, id, questions) => {
    await this.agent(`${this.ticketPath(key, id)}/post_round`, { claim_id: await this.claimed(key, id), questions });
  };

  reportFailure: Driver["reportFailure"] = async (key, id, report) => {
    await this.agent(`${this.ticketPath(key, id)}/report_failure`, { claim_id: await this.claimed(key, id), ...report });
  };

  finish: Driver["finish"] = async (key, id, summary, on) => {
    await this.agent(`${this.ticketPath(key, id)}/finish`, { claim_id: await this.claimed(key, id), summary, ...on });
  };

  addBlocker: Driver["addBlocker"] = async (key, id, blocker) => {
    await this.agent(`${this.ticketPath(key, id)}/add_blocker`, { blocker });
  };

  // ticket 35
  claim: Driver["claim"] = async (key, id, session_id) => {
    await this.claimed(key, id, session_id);
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
  close: Driver["close"] = () => this.c.close();
}
