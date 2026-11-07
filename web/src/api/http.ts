// The real Api: the HTTP API and the SSE feed of a running Code Central.
import { numberOf, type Api, type EventHandlers, type TicketAction, type TicketActions } from "./api";
import { ApiError, type ChangeEvent } from "./types";

export interface HttpApiOptions {
  // baseUrl is the service's origin; "" (the default) is the page's own.
  baseUrl?: string;
  // retryMs is how long the feed waits before reconnecting.
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
    act: <A extends TicketAction>(project: string, id: string, action: A, body: TicketActions[A]["body"]) =>
      call<TicketActions[A]["result"]>("POST", `${p(project)}/tickets/${encodeURIComponent(numberOf(id))}/${action}`, body),
    archiveProject: (project) => call("POST", `${p(project)}/archive_project`, {}),
    unarchiveProject: (project) => call("POST", `${p(project)}/unarchive_project`, {}),
    subscribe: (h) => subscribe(doFetch, base + "/api/events", retryMs, h),
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

// subscribe reads the SSE feed over fetch (so it runs in Node as well as
// the browser) and reconnects after retryMs whenever the stream ends.
function subscribe(doFetch: typeof fetch, url: string, retryMs: number, h: EventHandlers): () => void {
  let closed = false;
  let ctrl: AbortController | undefined;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let opened = false;

  const connect = async () => {
    ctrl = new AbortController();
    try {
      const res = await doFetch(url, { headers, signal: ctrl.signal });
      if (!res.ok || !res.body) throw new Error(`HTTP ${res.status}`);
      h.onOpen?.({ reconnect: opened });
      opened = true;
      await readEvents(res.body, h.onEvent);
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

async function readEvents(body: ReadableStream<Uint8Array>, onEvent: (e: ChangeEvent) => void) {
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
      let name = "";
      let data = "";
      for (const line of block.split("\n")) {
        if (line.startsWith("event:")) name = line.slice(6).trim();
        else if (line.startsWith("data:")) data += line.slice(5).trim();
      }
      if (name && data) onEvent({ type: name, ...JSON.parse(data) } as ChangeEvent);
    }
  }
}
