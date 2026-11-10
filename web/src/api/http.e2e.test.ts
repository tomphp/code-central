// The real half of the contract suite (`make e2e`): createHttpApi against
// a real `code-central serve --store <tmpdir>`, a fresh one per test, from
// the already-built binary in CODE_CENTRAL_BIN. It never binds port 7733.
import { serve } from "../e2e/serve";
import { apiContract, type Driver } from "./contract";
import { createHttpApi } from "./http";

apiContract("HTTP API against code-central serve", async (): Promise<Driver> => {
  const svc = await serve();
  // Transcript streams go through a fetch the driver can cut, as a flaky
  // connection would (ticket 110).
  const streams = new Set<AbortController>();
  const cuttable: typeof fetch = (input, init) => {
    if (!String(input).endsWith("/transcript/stream")) return fetch(input, init);
    const cut = new AbortController();
    streams.add(cut);
    init?.signal?.addEventListener("abort", () => cut.abort());
    return fetch(input, { ...init, signal: cut.signal });
  };
  const api = createHttpApi({ baseUrl: svc.url, retryMs: 50, fetch: cuttable });
  // The driver plays the agents over the same HTTP API.
  const agent = svc.agent;
  // integration is a write as the integration Actor (ticket 42).
  const integration = async (path: string, body: unknown) => {
    const res = await fetch(svc.url + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "integration" },
      body: JSON.stringify(body),
    });
    const v = await res.json();
    if (!res.ok) throw new Error(`${path}: ${JSON.stringify(v)}`);
    return v;
  };
  const specs = new Map<string, string>();
  return {
    api,
    createProject: async (key, display_name) => {
      await agent("/api/projects/create_project", { key, display_name });
      specs.set(key, (await agent(`/api/p/${key}/create_spec`, { title: "Contract", body: "" })).id);
    },
    createTicket: async (key, title, state) =>
      (await agent(`/api/p/${key}/create_ticket`, { spec: specs.get(key), title, state })).id,
    postRound: async (key, id, questions) => {
      const { claim_id } = await agent(`/api/p/${key}/claim`, { ticket: id, session_id: "contract-session" });
      await agent(`/api/p/${key}/tickets/${id.split("-").pop()}/post_round`, { claim_id, questions });
    },
    reportFailure: async (key, id, report) => {
      const { claim_id } = await agent(`/api/p/${key}/claim`, { ticket: id, session_id: "contract-session" });
      await agent(`/api/p/${key}/tickets/${id.split("-").pop()}/report_failure`, { claim_id, ...report });
    },
    finish: async (key, id, summary, on) => {
      const { claim_id } = await agent(`/api/p/${key}/claim`, { ticket: id, session_id: "contract-session" });
      await agent(`/api/p/${key}/tickets/${id.split("-").pop()}/finish`, { claim_id, summary, ...on });
    },
    addBlocker: async (key, id, blocker) => {
      await agent(`/api/p/${key}/tickets/${id.split("-").pop()}/add_blocker`, { blocker });
    },
    // ticket 35
    claim: async (key, id, session_id) => {
      await agent(`/api/p/${key}/claim`, { ticket: id, session_id });
    },
    // ticket 42
    linkTicket: async (key, title, link, external_state) => {
      const { id } = await integration(`/api/p/${key}/integration/create_linked_ticket`, { spec: specs.get(key), title, link });
      await integration(`/api/p/${key}/integration/set_external_state`, { target: id, external_state });
      return id;
    },
    // code-central-287
    incomingIssue: async (key, spec, title, link) =>
      (await integration(`/api/p/${key}/integration/create_linked_ticket`, { spec, title, link })).id,
    // ticket 109: as a Factory; the endpoint answers 204 with no body.
    postSessionEvents: async (session_id, events) => {
      const res = await fetch(svc.url + "/api/session-events", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id, events }),
      });
      if (res.status !== 204) throw new Error(`session-events: ${res.status}`);
    },
    // Spec code-central-201: one JSON-RPC tools/call to the Project's MCP
    // endpoint, which records it as Usage.
    mcpCall: async (key, op, args) => {
      const res = await fetch(`${svc.url}/p/${key}/mcp`, {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json, text/event-stream" },
        body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: op, arguments: args } }),
      });
      const { result } = await res.json();
      const text: string = result.content[0].text;
      return { text, error: result.isError ? JSON.parse(text).code : undefined };
    },
    // Spec code-central-201: as a Factory; the endpoint answers 204 with no body.
    postUsage: async (session_id, responses) => {
      const res = await fetch(svc.url + "/api/usage", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id, responses }),
      });
      if (res.status !== 204) throw new Error(`usage: ${res.status}`);
    },
    // code-central-251: as a Factory
    reportPushRemote: async (key, url) => {
      await agent(`/api/p/${key}/report_push_remote`, { url });
    },
    // code-central-253: as the CI Watcher
    reportCIWatcherStatus: async (key, status) => {
      await agent(`/api/p/${key}/report_ci_watcher_status`, status);
    },
    // code-central-256: as the Factory and the CI Watcher
    reportPushedCommit: async (key, id, commit) => {
      await agent(`/api/p/${key}/tickets/${id.split("-").pop()}/report_pushed_commit`, { commit });
    },
    reportCIRuns: async (key, runs) => {
      await agent(`/api/p/${key}/report_ci_runs`, { runs });
    },
    logOutOfGH: svc.logOutOfGH,
    letGHTriage: svc.letGHTriage,
    dropTranscriptStreams: () => {
      for (const cut of streams) cut.abort();
      streams.clear();
    },
    close: svc.stop,
  };
});
