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
    close: svc.stop,
  };
});
