// Helpers for the UI's end-to-end tests (`make e2e`, *.e2e.test.ts): a
// real `code-central serve --store <tmpdir> --no-notify` from the built
// binary in CODE_CENTRAL_BIN, on a free loopback port (never 7733), and
// agent requests over its HTTP API. The service finds the fake gh built
// in CODE_CENTRAL_FAKE_GH_BIN first on its PATH, never the real one.
import { spawn } from "node:child_process";
import { copyFile, mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { delimiter, join } from "node:path";

export function binary(): string {
  const bin = process.env.CODE_CENTRAL_BIN;
  if (!bin) throw new Error("CODE_CENTRAL_BIN must name a built code-central binary (make e2e sets it)");
  return bin;
}

function fakeGHBinary(): string {
  const bin = process.env.CODE_CENTRAL_FAKE_GH_BIN;
  if (!bin) throw new Error("CODE_CENTRAL_FAKE_GH_BIN must name the built fake gh (make e2e sets it)");
  return bin;
}

export interface Service {
  url: string;
  // agent makes one POST as an agent, as an MCP tool call or the CLI would.
  agent(path: string, body: unknown): Promise<any>;
  // human makes one POST as the human, as the UI does.
  human(path: string, body: unknown): Promise<any>;
  // logOutOfGH makes the fake gh answer gh auth status as a gh with no
  // login does.
  logOutOfGH(): Promise<void>;
  stop(): Promise<void>;
}

// fakeGH puts the fake gh, as gh, in a folder of its own under dir, and
// returns that folder and the one it keeps its Script and calls in.
async function fakeGH(dir: string): Promise<{ bin: string; data: string }> {
  const bin = join(dir, "bin");
  const data = join(dir, "data");
  await mkdir(bin);
  await mkdir(data);
  await copyFile(fakeGHBinary(), join(bin, process.platform === "win32" ? "gh.exe" : "gh"));
  return { bin, data };
}

export async function serve(): Promise<Service> {
  const store = await mkdtemp(join(tmpdir(), "cc-ui-e2e-"));
  const ghDir = await mkdtemp(join(tmpdir(), "cc-ui-e2e-gh-"));
  const gh = await fakeGH(ghDir);
  // Windows names it Path; a second PATH beside it would be ambiguous.
  const pathKey = Object.keys(process.env).find((k) => k.toUpperCase() === "PATH") ?? "PATH";
  const proc = spawn(binary(), ["serve", "--store", store, "--no-notify"], {
    env: {
      ...process.env,
      CODE_CENTRAL_LISTEN: "127.0.0.1:0",
      [pathKey]: gh.bin + delimiter + (process.env[pathKey] ?? ""),
      FAKE_GH_DIR: gh.data,
    },
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
    agent: (path, body) => post(url, "agent", path, body),
    human: (path, body) => post(url, "human", path, body),
    logOutOfGH: () =>
      writeFile(
        join(gh.data, "answers.json"),
        JSON.stringify([
          { args: ["auth", "status"], exit: 1, stderr: "You are not logged into any GitHub hosts. To log in, run: gh auth login\n" },
        ]),
      ),
    stop: async () => {
      const exited = new Promise((r) => proc.on("exit", r));
      proc.kill("SIGTERM");
      await exited;
      // On Windows, a gh the service ran may still hold its files for a
      // moment after the service exits, so rm retries while they're locked.
      await rm(store, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
      await rm(ghDir, { recursive: true, force: true, maxRetries: 10, retryDelay: 100 });
    },
  };
}

// post makes one POST to the service as actor, failing on a refusal.
async function post(url: string, actor: "agent" | "human", path: string, body: unknown): Promise<any> {
  const res = await fetch(url + path, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Code-Central-Actor": actor },
    body: JSON.stringify(body),
  });
  const v = await res.json();
  if (!res.ok) throw new Error(`${path}: ${JSON.stringify(v)}`);
  return v;
}
