// The real Api: the HTTP API and the SSE feeds of a running Code Central.
import {
  numberOf,
  type Api,
  type TicketAction,
  type TicketActions,
  type ChatHandlers,
  type TranscriptHandlers,
} from "./api";
import { OPERATIONS, type OperationInputs, type OperationName } from "./wire.gen";
import { ApiError, type ChangeEvent, type ChatOutcome, type Conversation, type ChatItem, type ChatMessage, type HeldSessionEvent } from "./types";

export interface HttpApiOptions {
  // baseUrl is the service's origin; "" (the default) is the page's own.
  baseUrl?: string;
  // retryMs is how long a feed waits before reconnecting.
  retryMs?: number;
  fetch?: typeof fetch;
}

// OperationEntry is an Operation Table entry as operate reads it: the
// table's literal types widened to one shape.
interface OperationEntry {
  method: "GET" | "POST";
  path: string;
  bind: Record<string, string>;
  query: Record<string, string>;
}

// Every Ticket action is an operation in the Operation Table: an action
// file that names something else fails to type-check here.
const actionsAreOperations: [Exclude<TicketAction, OperationName>] extends [never] ? true : never = true;
void actionsAreOperations;

// The UI is the human: only it sends this Actor.
const headers = { "X-Code-Central-Actor": "human" };

export function createHttpApi(opts: HttpApiOptions = {}): Api {
  const base = opts.baseUrl ?? "";
  const retryMs = opts.retryMs ?? 1000;
  const doFetch = opts.fetch ?? ((...a: Parameters<typeof fetch>) => fetch(...a));

  async function call<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
    let res: Response;
    try {
      res = await doFetch(base + path, {
        method,
        headers: body === undefined ? headers : { ...headers, "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch {
      throw new ApiError(0, "service_unavailable", "Code Central isn't reachable: is `code-central serve` running?");
    }
    const text = await res.text();
    if (res.ok) return JSON.parse(text) as T;
    throw toError(res.status, text);
  }

  // operate sends the operation name's request for input: the path and
  // query from the Operation Table, the body from the input.
  function operate<N extends OperationName, T>(name: N, project: string, input: OperationInputs[N]): Promise<T> {
    const op: OperationEntry = OPERATIONS[name];
    const values = input as Record<string, unknown>;
    let path = op.path.replace("{key}", encodeURIComponent(project));
    for (const [placeholder, field] of Object.entries(op.bind)) {
      path = path.replace(`{${placeholder}}`, encodeURIComponent(String(values[field])));
    }
    const { method } = op;
    if (method === "GET") {
      const q = new URLSearchParams();
      for (const [field, param] of Object.entries(op.query)) {
        const v = values[field];
        if (Array.isArray(v)) for (const item of v) q.append(param, String(item));
        else if (typeof v === "string" ? v !== "" : v === true) q.append(param, String(v));
      }
      return call("GET", path + (q.size ? `?${q}` : ""));
    }
    // The path carries what it binds, so the body doesn't.
    const bound = new Set(Object.values(op.bind));
    return call("POST", path, Object.fromEntries(Object.entries(values).filter(([field]) => !bound.has(field))));
  }
  const transcriptPath = (project: string, id: string, claim: number) =>
    `/api/projects/${encodeURIComponent(project)}/tickets/${encodeURIComponent(numberOf(id))}/claims/${claim}/transcript`;

  return {
    listProjects: () => operate("list_projects", "", {}),
    needsMe: () => operate("needs_me", "", {}),
    findTickets: (project, filter = {}) => operate("find_tickets", project, { state: filter.state }),
    getTicket: (project, id) => operate("get_ticket", project, { ticket: numberOf(id) }),
    getSpec: (project, spec) => operate("get_spec", project, { spec: numberOf(spec) }),
    findSpecs: (project, filter = {}) =>
      operate("find_specs", project, { requirements: filter.requirements, labels: filter.labels, finished: filter.finished }),
    submitRequirement: (project, text, map, skipGrilling, draft) =>
      operate("submit_requirement", project, {
        text,
        ...(map && { map }),
        ...(skipGrilling && { skip_grilling: true }),
        ...(draft && { draft }),
      }),
    submitMap: (project, text, draft) => operate("submit_map", project, { text, ...(draft && { draft }) }),
    submitPrototype: (project, spec, text) => operate("submit_prototype", project, { spec, text }),
    submitRetro: (project, target, note) => operate("submit_retro", project, { target, ...(note && { note }) }),
    submitSmallChange: (project, text, draft) => operate("submit_small_change", project, { text, ...(draft && { draft }) }),
    newTicket: (project, spec, text, draft) => operate("new_ticket", project, { spec, text, ...(draft && { draft }) }),
    specQuestions: (project, spec) => operate("get_spec_questions", project, { spec: numberOf(spec) }),
    act: <A extends TicketAction>(project: string, id: string, action: A, body: TicketActions[A]["body"]) =>
      operate<A, TicketActions[A]["result"]>(action, project, { ...body, ticket: numberOf(id) } as OperationInputs[A]),
    editSpec: (project, spec, change) => operate("edit_spec", project, { ...change, spec: numberOf(spec) }),
    archiveProject: (project) => operate("archive_project", project, {}),
    unarchiveProject: (project) => operate("unarchive_project", project, {}),
    renameProject: (project, display_name) => operate("rename_project", project, { display_name }),
    setLandingMethod: (project, landing) => operate("set_landing_method", project, landing),
    setCITracking: (project, ci) => operate("set_ci_tracking", project, { ...ci, full: true }),
    setAutoFix: (project, autoFix) => operate("set_auto_fix", project, { ...autoFix, full: true }),
    setRepositoryCI: (project, name, ci) => operate("set_repository_ci", project, { ...ci, name, full: true }),
    // Only the settings themselves: a Project's read adds spec and watcher.
    setGitHubIssues: (project, { on, repositories, label, new_issues_need_me }) =>
      operate("set_github_issues", project, { on, repositories, label, new_issues_need_me, full: true }),
    // Only the settings themselves: a Project's read adds spec.
    setAzureBoards: (project, { on, mode, query_url, project_url, where, pat_env, new_issues_need_me, hide_code_central, branch_format, work_item_states }) =>
      operate("set_azure_boards", project, { on, mode, query_url, project_url, where, pat_env, new_issues_need_me, hide_code_central, branch_format, work_item_states, full: true }),
    setModels: (project, models) =>
      operate("set_models", project, { default: models.default, chat: models.chat ?? { model: "", effort: "" }, kinds: models.kinds, full: true }),
    setSubagentExploration: (project, on) => operate("set_subagent_exploration", project, { on, full: true }),
    setModelAssignment: (project, on) => operate("set_model_assignment", project, { on, full: true }),
    setAutoAccept: (project, kinds) => operate("set_auto_accept", project, { kinds, full: true }),
    setFocusKinds: (project, off) => operate("set_focus_kinds", project, { off, full: true }),
    setCheckCommand: (project, check_command) => operate("set_check_command", project, { check_command, full: true }),
    addCIRule: (project, rule) => operate("add_ci_rule", project, { ...rule, full: true }),
    editCIRule: (project, spec, rule) => operate("edit_ci_rule", project, { ...rule, spec, full: true }),
    removeCIRule: (project, spec) => operate("remove_ci_rule", project, { spec, full: true }),
    addRepository: (project, repository) => operate("add_repository", project, { ...repository, full: true }),
    editRepository: (project, name, change) => operate("edit_repository", project, { ...change, name, full: true }),
    removeRepository: (project, name) => operate("remove_repository", project, { name, full: true }),
    setLandingOrder: (project, names) => operate("set_landing_order", project, { names, full: true }),
    unlink: (project, target) => operate("unlink", project, { target }),
    setTags: (project, target, tags) => operate("set_tags", project, { target, tags }),
    removeTags: (project, target, keys) => operate("remove_tags", project, { target, keys }),
    addLabels: (project, target, labels) => operate("add_labels", project, { target, labels }),
    removeLabels: (project, target, labels) => operate("remove_labels", project, { target, labels }),
    options: () => operate("get_options", "", {}),
    setOptions: (change) => operate("set_options", "", change),
    usage: (filter = {}) => {
      const q = new URLSearchParams();
      if (filter.project) q.set("project", filter.project);
      if (filter.days) q.set("days", String(filter.days));
      const qs = q.toString();
      return call("GET", `/api/usage${qs ? `?${qs}` : ""}`);
    },
    ciOverview: (filter = {}) => {
      const q = new URLSearchParams();
      if (filter.project) q.set("project", filter.project);
      if (filter.running) q.set("running", "true");
      return call("GET", `/api/ci${q.toString() ? `?${q}` : ""}`);
    },
    sessionUsage: ({ project, session, agent }) => {
      const q = new URLSearchParams({ project, session });
      if (agent) q.set("agent", agent);
      return call("GET", `/api/usage/session?${q}`);
    },
    storage: (project) => operate("get_storage", "", { project }),
    subscribe: (h) =>
      follow(doFetch, base + "/api/events", retryMs, {
        onOpen: h.onOpen,
        onFrame: (f) => {
          if (f.data) h.onEvent({ type: f.name, ...JSON.parse(f.data) } as ChangeEvent);
        },
      }),
    transcript: (project, id, claim) => call("GET", transcriptPath(project, id, claim)),
    subscribeTranscript: (project, id, claim, h) => subscribeTranscript(doFetch, base + transcriptPath(project, id, claim) + "/stream", retryMs, h),
    getChat: (project) => operate("get_chat", project, {}),
    startConversation: (project, pick) => operate("start_conversation", project, pick),
    sendChatMessage: (project, text) => operate("send_chat_message", project, { text }),
    stopChatReply: (project) => operate("stop_chat_reply", project, {}),
    subscribeChat: (project, h) => subscribeChat(doFetch, base + `/api/p/${encodeURIComponent(project)}/chat/stream`, retryMs, h),
  };
}

// toError is a refused request's ApiError, from its status and body.
export function toError(status: number, text: string): ApiError {
  try {
    const e = JSON.parse(text);
    if (typeof e?.code === "string") return new ApiError(status, e.code, e.message ?? "", e.field, e.allowed_actions, e.fix);
  } catch {
    // Not the JSON error shape: the Guard's 403 or a proxy's error page.
  }
  return new ApiError(status, "internal", text.trim() || `HTTP ${status}`);
}

// subscribeTranscript follows a Transcript stream, sending the last seq
// it saw as Last-Event-ID on each reconnect, so the stream resumes with no
// gap and no repeats. A host frame, from a service holding none, ends it
// for good: it doesn't reconnect.
function subscribeTranscript(doFetch: typeof fetch, url: string, retryMs: number, h: TranscriptHandlers): () => void {
  let last: string | undefined;
  const stop = follow(doFetch, url, retryMs, {
    headers: (): Record<string, string> => (last ? { "Last-Event-ID": last } : {}),
    onOpen: h.onOpen,
    onFrame: (f) => {
      if (f.name === "tail") h.onTail?.();
      else if (f.name === "host" && f.data) {
        stop();
        h.onHost?.((JSON.parse(f.data) as { host: string }).host);
      } else if (f.name === "session_event" && f.data) {
        if (f.id) last = f.id;
        h.onEvent(JSON.parse(f.data) as HeldSessionEvent);
      }
    },
  });
  return stop;
}

// subscribeChat follows a Conversation stream, sending the last id it saw as
// Last-Event-ID on each reconnect, so the stream resumes with no gap and no
// repeats. A frame with no id (replaced) leaves it as it was.
function subscribeChat(doFetch: typeof fetch, url: string, retryMs: number, h: ChatHandlers): () => void {
  let last: string | undefined;
  return follow(doFetch, url, retryMs, {
    headers: (): Record<string, string> => (last ? { "Last-Event-ID": last } : {}),
    onOpen: h.onOpen,
    onFrame: (f) => {
      if (f.id) last = f.id;
      const data = f.data ? JSON.parse(f.data) : undefined;
      switch (f.name) {
        case "conversation":
          h.onConversation(data as Conversation);
          break;
        case "message":
          h.onMessage?.(data as ChatMessage);
          break;
        case "item":
          h.onItem?.(data as ChatItem);
          break;
        case "outcome":
          h.onOutcome?.(data as { outcome: ChatOutcome; error?: string });
          break;
        case "session":
          h.onSession?.(data as { session_id: string; resume_command: string });
          break;
        case "replaced":
          h.onReplaced?.();
          break;
      }
    },
  });
}

// A Frame is one SSE event: its event:, id: and data: fields.
interface Frame {
  name: string;
  id: string;
  data: string;
}

interface Follower {
  // headers are the extra request headers for each (re)connect.
  headers?(): Record<string, string>;
  onOpen?(info: { reconnect: boolean }): void;
  onFrame(f: Frame): void;
}

// follow reads an SSE stream over fetch (so it runs in Node as well as
// the browser) and reconnects after retryMs whenever the stream ends.
function follow(doFetch: typeof fetch, url: string, retryMs: number, h: Follower): () => void {
  let closed = false;
  let ctrl: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let opened = false;

  const connect = async () => {
    ctrl = new AbortController();
    try {
      const res = await doFetch(url, { headers: { ...headers, ...h.headers?.() }, signal: ctrl.signal });
      if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
      h.onOpen?.({ reconnect: opened });
      opened = true;
      await readFrames(res.body, h.onFrame);
    } catch {
      // Dropped or refused: retry below.
    }
    if (!closed) timer = setTimeout(connect, retryMs);
  };
  connect();
  return () => {
    closed = true;
    clearTimeout(timer);
    ctrl?.abort();
  };
}

async function readFrames(body: ReadableStream<Uint8Array>, onFrame: (f: Frame) => void) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) return;
    buf += decoder.decode(value, { stream: true });
    let end: number;
    while ((end = buf.indexOf("\n\n")) >= 0) {
      const block = buf.slice(0, end);
      buf = buf.slice(end + 2);
      const f: Frame = { name: "", id: "", data: "" };
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) f.name = line.slice(6).trim();
        else if (line.startsWith("id:")) f.id = line.slice(3).trim();
        else if (line.startsWith("data:")) f.data += line.slice(5).trim();
      }
      if (f.name) onFrame(f);
    }
  }
}
