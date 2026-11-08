// The real half of the contract suite (`make e2e`): createHttpApi against
// a real `code-central serve --store <tmpdir>`, a fresh one per test, from
// the already-built binary in CODE_CENTRAL_BIN. It never binds port 7733.
import { serve } from "../e2e/serve";
import { apiContract, type Driver } from "./contract";
import { createHttpApi } from "./http";

apiContract("HTTP API against code-central serve", async (): Promise<Driver> => {
  const svc = await serve();
  const api = createHttpApi({ baseUrl: svc.url, retryMs: 50 });
  // The driver plays the agents over the same HTTP API.
  const agent = svc.agent;
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
    // ticket 42: as the integration Actor
    linkTicket: async (key, title, link, external_state) => {
      const as = async (path: string, body: unknown) => {
        const res = await fetch(svc.url + path, {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "integration" },
          body: JSON.stringify(body),
        });
        const v = await res.json();
        if (!res.ok) throw new Error(`${path}: ${JSON.stringify(v)}`);
        return v;
      };
      const { id } = await as(`/api/p/${key}/integration/create_linked_ticket`, { spec: specs.get(key), title, link });
      await as(`/api/p/${key}/integration/set_external_state`, { target: id, external_state });
      return id;
    },
    // ticket 109: as a Factory; the endpoint answers 204 with no body.
    postSessionEvents: async (session_id, events) => {
      const res = await fetch(svc.url + "/api/session-events", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ session_id, events }),
      });
      if (res.status !== 204) throw new Error(`session-events: ${res.status}`);
    },
    close: svc.stop,
  };
});
