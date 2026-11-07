// The real half of the contract suite (`make e2e`): createHttpApi against
// a real `code-central serve --store <tmpdir>`, a fresh one per test, from
// the already-built binary in CODE_CENTRAL_BIN. It never binds port 7733.
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { apiContract, type Driver } from "./contract";
import { createHttpApi } from "./http";

const bin = process.env.CODE_CENTRAL_BIN;
if (!bin) throw new Error("CODE_CENTRAL_BIN must name a built code-central binary (make e2e sets it)");

async function serve(): Promise<{ url: string; stop: () => Promise<void> }> {
  const store = await mkdtemp(join(tmpdir(), "cc-ui-contract-"));
  const proc = spawn(bin!, ["serve", "--store", store, "--no-notify"], {
    env: { ...process.env, CODE_CENTRAL_LISTEN: "127.0.0.1:0" },
    stdio: ["ignore", "pipe", "inherit"],
  });
  const url = await new Promise<string>((resolve, reject) => {
    let out = "";
    proc.stdout!.on("data", (b) => {
      out += b;
      const m = out.match(/listening on \[([^\] ]+)/);
      if (m) resolve(`http://${m[1]}`);
    });
    proc.on("exit", (code) => reject(new Error(`serve exited ${code}: ${out}`)));
  });
  return {
    url,
    stop: async () => {
      const exited = new Promise((r) => proc.on("exit", r));
      proc.kill("SIGTERM");
      await exited;
      await rm(store, { recursive: true, force: true });
    },
  };
}

apiContract("HTTP API against code-central serve", async (): Promise<Driver> => {
  const svc = await serve();
  const api = createHttpApi({ baseUrl: svc.url, retryMs: 50 });
  // The driver plays the agents over the same HTTP API.
  const agent = async (path: string, body: unknown) => {
    const res = await fetch(svc.url + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "agent" },
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
    close: svc.stop,
  };
});
