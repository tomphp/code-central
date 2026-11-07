// Helpers for the UI's end-to-end tests (`make e2e`, *.e2e.test.ts): a
// real `code-central serve --store <tmpdir> --no-notify` from the built
// binary in CODE_CENTRAL_BIN, on a free loopback port (never 7733), and
// agent requests over its HTTP API.
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

export function binary(): string {
  const bin = process.env.CODE_CENTRAL_BIN;
  if (!bin) throw new Error("CODE_CENTRAL_BIN must name a built code-central binary (make e2e sets it)");
  return bin;
}

export interface Service {
  url: string;
  // agent makes one POST as an agent, as an MCP tool call or the CLI would.
  agent(path: string, body: unknown): Promise<any>;
  stop(): Promise<void>;
}

export async function serve(): Promise<Service> {
  const store = await mkdtemp(join(tmpdir(), "cc-ui-e2e-"));
  const proc = spawn(binary(), ["serve", "--store", store, "--no-notify"], {
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
    agent: async (path, body) => {
      const res = await fetch(url + path, {
        method: "POST",
        headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "agent" },
        body: JSON.stringify(body),
      });
      const v = await res.json();
      if (!res.ok) throw new Error(`${path}: ${JSON.stringify(v)}`);
      return v;
    },
    stop: async () => {
      const exited = new Promise((r) => proc.on("exit", r));
      proc.kill("SIGTERM");
      await exited;
      await rm(store, { recursive: true, force: true });
    },
  };
}
