// The real Api: the HTTP API and the SSE feeds of a running Code Central.
import {
  numberOf,
  type Api,
  type EventHandlers,
  type TicketAction,
  type TicketActions,
  type TranscriptHandlers,
} from "./api";
import { ApiError, type ChangeEvent, type HeldSessionEvent } from "./types";

export interface HttpApiOptions {
  // baseUrl is the service's origin; "" (the default) is the page's own.
  baseUrl?: string;
  // retryMs is how long a feed waits before reconnecting.
  retryMs?: number;
  fetch?: typeof fetch;
}

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

  const p = (project: string) => `/api/p/${encodeURIComponent(project)}`;
  const transcriptPath = (project: string, id: string, claim: number) =>
    `/api/projects/${encodeURIComponent(project)}/tickets/${encodeURIComponent(numberOf(id))}/claims/${claim}/transcript`;

  return {
    listProjects: () => call("GET", "/api/projects"),
    needsMe: () => call("GET", "/api/needs-me"),
    findTickets: (project, filter = {}) => {
      const q = new URLSearchParams();
      for (const s of filter.state ?? []) q.append("state", s);
      const qs = q.toString();
      return call("GET", `${p(project)}/tickets${qs ? `?${qs}` : ""}`);
    },
    getTicket: (project, id) => call("GET", `${p(project)}/tickets/${encodeURIComponent(numberOf(id))}`),
    getSpec: (project, spec) => call("GET", `${p(project)}/specs/${encodeURIComponent(numberOf(spec))}`),
    findSpecs: (project, filter = {}) => {
      const q = new URLSearchParams();
      if (filter.requirements) q.set("requirements", "true");
      if (filter.finished) q.set("finished", "true");
      const qs = q.toString();
      return call("GET", `${p(project)}/specs${qs ? `?${qs}` : ""}`);
    },
    submitRequirement: (project, text) => call("POST", `${p(project)}/submit_requirement`, { text }),
    specQuestions: (project, spec) => call("GET", `${p(project)}/specs/${encodeURIComponent(numberOf(spec))}/questions`),
    act: <A extends TicketAction>(project: string, id: string, action: A, body: TicketActions[A]["body"]) =>
      call<TicketActions[A]["result"]>("POST", `${p(project)}/tickets/${encodeURIComponent(numberOf(id))}/${action}`, body),
    archiveProject: (project) => call("POST", `${p(project)}/archive_project`, {}),
    unarchiveProject: (project) => call("POST", `${p(project)}/unarchive_project`, {}),
    renameProject: (project, display_name) => call("POST", `${p(project)}/rename_project`, { display_name }),
    unlink: (project, target) => call("POST", `${p(project)}/unlink`, { target }),
    subscribe: (h) =>
      follow(doFetch, base + "/api/events", retryMs, {
        onOpen: h.onOpen,
        onFrame: (f) => {
          if (f.data) h.onEvent({ type: f.name, ...JSON.parse(f.data) } as ChangeEvent);
        },
      }),
    transcript: (project, id, claim) => call("GET", transcriptPath(project, id, claim)),
    subscribeTranscript: (project, id, claim, h) => subscribeTranscript(doFetch, base + transcriptPath(project, id, claim) + "/stream", retryMs, h),
  };
}

function toError(status: number, text: string): ApiError {
  try {
    const e = JSON.parse(text);
    if (typeof e?.code === "string") return new ApiError(status, e.code, e.message ?? "", e.field, e.allowed_actions);
  } catch {
    // Not the JSON error shape: the Guard's 403 or a proxy's error page.
  }
  return new ApiError(status, "internal", text.trim() || `HTTP ${status}`);
}

// subscribeTranscript follows a Transcript stream, sending the last seq
// it saw as Last-Event-ID on each reconnect, so the stream resumes with no
// gap and no repeats.
function subscribeTranscript(doFetch: typeof fetch, url: string, retryMs: number, h: TranscriptHandlers): () => void {
  let last: string | undefined;
  return follow(doFetch, url, retryMs, {
    headers: (): Record<string, string> => (last ? { "Last-Event-ID": last } : {}),
    onOpen: h.onOpen,
    onFrame: (f) => {
      if (f.name === "tail") h.onTail?.();
      else if (f.name === "session_event" && f.data) {
        if (f.id) last = f.id;
        h.onEvent(JSON.parse(f.data) as HeldSessionEvent);
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
