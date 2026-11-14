// The WASM Core (Spec code-central-580): the real Core and HTTP API, built
// to WebAssembly by `make corewasm` (cmd/corewasm), for the UI's tests
// and stories to run on in place of a service.
//
// core.wasm and Go's wasm_exec.js load once per process: from disk under
// vitest, over HTTP in Storybook. Each World is a fresh Core with its own
// in-memory store and fake clock. Its api is the real createHttpApi over
// the world's fetch, which serves each request inside WebAssembly and
// streams its response, so the live feeds go through createHttpApi's own
// SSE code. A world holds no seeded state: tests and stories build it
// with the scenario builder (scenario.ts), through real operations.
//
// Every world made is kept until dropWorlds, which the shared test setup
// calls after each test; a story's world lives as long as the story.
import type { Api } from "../api";
import { createHttpApi } from "../http";

// CoreModule is what cmd/corewasm sets on globalThis.codeCentralCore.
interface CoreModule {
  newWorld(startISO: string): number;
  fetch(id: number, method: string, url: string, headers: [string, string][], body?: string, signal?: AbortSignal): Promise<Response>;
  now(id: number): string;
  advance(id: number, ms: number): void;
  setCICheck(id: number, resultJSON: string): void;
  setIssuesCheck(id: number, resultJSON: string): void;
  setPullRequestCheck(id: number, resultJSON: string): void;
  reportWatcherCrash(id: number, kind: string, key: string, restarts: number, lastLine: string, restartAtISO: string): void;
  dropStreams(id: number): void;
  transcriptStreams(id: number): number;
  drop(id: number): void;
}

declare global {
  // eslint-disable-next-line no-var
  var codeCentralCore: CoreModule | undefined;
  // eslint-disable-next-line no-var
  var Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> };
}

// CheckResult is what a stand-in gh check answers (core.WatcherCheckResult):
// a problem, with the reason and fix Core refuses with, or nothing to pass.
export interface CheckResult {
  problem?: string;
  reason?: string;
  fix?: string;
}

// WatcherCrash is the Watcher supervisor's report of a crashed Watcher.
export interface WatcherCrash {
  restarts: number;
  lastLine: string;
  restartAt: Date;
}

// START is when a world's clock starts unless told otherwise.
export const START = "2026-11-07T09:00:00Z";

// READS are the Api's reads, by name, which actions leaves out.
const READS: ReadonlySet<string> = new Set<keyof Api>([
  "listProjects", "needsMe", "findTickets", "getTicket", "getSpec", "findSpecs", "specQuestions", "options", "transcript", "usage", "sessionUsage", "ciOverview",
]);

let loading: Promise<CoreModule> | undefined;

// loadCore starts core.wasm the first time it is called, and returns it.
function loadCore(): Promise<CoreModule> {
  loading ??= (async () => {
    await import("./wasm_exec.js");
    const go = new globalThis.Go();
    const { instance } = await WebAssembly.instantiate(await wasmBytes(), go.importObject);
    void go.run(instance);
    const core = globalThis.codeCentralCore;
    if (!core) throw new Error("core.wasm did not start");
    return core;
  })();
  return loading;
}

// wasmBytes reads core.wasm: from disk under vitest, which gives a
// module its folder, and over HTTP in Storybook.
async function wasmBytes(): Promise<BufferSource> {
  if (import.meta.dirname) {
    // Named apart so Storybook's bundler leaves it alone.
    const fs = "node:fs/promises";
    const { readFile } = (await import(/* @vite-ignore */ fs)) as typeof import("node:fs/promises");
    return readFile(`${import.meta.dirname}/core.wasm`);
  }
  const res = await fetch(new URL("./core.wasm", import.meta.url));
  if (!res.ok) throw new Error(`core.wasm: HTTP ${res.status}; run make corewasm`);
  return res.arrayBuffer();
}

const worlds = new Set<World>();

// newWorld is a fresh WASM Core whose clock reads start.
export async function newWorld(start: string = START): Promise<World> {
  const core = await loadCore();
  const w = new World(core, core.newWorld(start));
  worlds.add(w);
  return w;
}

// dropWorlds drops every world made so far.
export function dropWorlds() {
  for (const w of worlds) w.drop();
}

export class World {
  // fetch serves a request inside the world: the way in for createHttpApi
  // and the Driver. It takes a URL, not a Request, and a string body.
  readonly fetch: typeof fetch;
  // api is the real createHttpApi over fetch, recording each method called.
  readonly api: Api;
  // calls records each Api method called by name, an action by its own.
  readonly calls: string[] = [];
  private dropped = false;

  constructor(
    private readonly core: CoreModule,
    private readonly id: number,
  ) {
    this.fetch = async (input, init = {}) => {
      // A request still on its way when its test ends fails as a fetch
      // with no service behind it does.
      if (this.dropped) throw new TypeError("fetch failed: the world has been dropped");
      if (input instanceof Request || (init.body != null && typeof init.body !== "string")) {
        throw new Error("a world's fetch takes a URL and a string body");
      }
      return await core.fetch(id, init.method ?? "GET", String(input), [...new Headers(init.headers)], init.body ?? undefined, init.signal ?? undefined);
    };
    const api = createHttpApi({ fetch: this.fetch, retryMs: 50 });
    this.api = new Proxy(api, {
      get: (t, k: string) => {
        const v = t[k as keyof Api];
        if (typeof v !== "function" || k.startsWith("subscribe")) return v;
        return (...a: unknown[]) => {
          this.calls.push(k === "act" ? String(a[2]) : k);
          return (v as (...a: unknown[]) => unknown)(...a);
        };
      },
    });
  }

  // actions is calls without the reads: the writes made, in order.
  actions(): string[] {
    return this.calls.filter((c) => !READS.has(c));
  }

  // now is the time on the world's clock, bound so App can take it as
  // its own clock: <App api={w.api} now={w.now} />.
  readonly now = (): Date => new Date(this.live().now(this.id));

  // advance moves the world's clock forward by ms.
  advance(ms: number) {
    this.live().advance(this.id, ms);
  }

  // setCICheck makes the gh check CI Tracking runs answer res, as a gh
  // that can or can't watch would; with none set, it passes. So do
  // setIssuesCheck for GitHub Issues and setPullRequestCheck for the Pull
  // Request Landing Method.
  setCICheck(res: CheckResult = {}) {
    this.live().setCICheck(this.id, JSON.stringify(res));
  }

  setIssuesCheck(res: CheckResult = {}) {
    this.live().setIssuesCheck(this.id, JSON.stringify(res));
  }

  setPullRequestCheck(res: CheckResult = {}) {
    this.live().setPullRequestCheck(this.id, JSON.stringify(res));
  }

  // reportWatcherCrash reports that key's Watcher of kind crashed, as the
  // service's Watcher supervisor does.
  reportWatcherCrash(kind: "ci" | "issues" | "pull_request", key: string, crash: WatcherCrash) {
    this.live().reportWatcherCrash(this.id, kind, key, crash.restarts, crash.lastLine, crash.restartAt.toISOString());
  }

  // dropStreams ends every open stream, change notices and Transcript
  // streams alike, as a dropped connection would; each reconnects.
  dropStreams() {
    this.live().dropStreams(this.id);
  }

  // transcriptStreams counts the Transcript streams open now.
  transcriptStreams(): number {
    return this.live().transcriptStreams(this.id);
  }

  // live is the module while the world is: core.wasm stops, for every
  // world, when given a dropped one's id.
  private live(): CoreModule {
    if (this.dropped) throw new Error("the world has been dropped");
    return this.core;
  }

  // drop ends the world and its streams and frees it; it takes no more
  // requests. Dropping it again does nothing.
  drop() {
    if (this.dropped) return;
    this.dropped = true;
    worlds.delete(this);
    this.core.drop(this.id);
  }
}
