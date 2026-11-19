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
  setAzureBoardsCheck(id: number, resultJSON: string): void;
  setPullRequestCheck(id: number, resultJSON: string): void;
  reportWatcherCrash(id: number, kind: string, key: string, restarts: number, lastLine: string, restartAtISO: string): void;
  holdWatcherLease(id: number, key: string, host: string): void;
  dropStreams(id: number): void;
  transcriptStreams(id: number): number;
  settled(id: number, events: number, transcripts: number, chats: number): string;
  drop(id: number): void;
}

declare global {
  var codeCentralCore: CoreModule | undefined;
  var Go: new () => { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> };
}

// CheckResult is what a stand-in gh check answers (core.WatcherCheckResult):
// a problem, with the reason and fix Core refuses with, or nothing to pass.
export interface CheckResult {
  problem?: string;
  reason?: string;
  fix?: string;
  // repositories are the failing repositories of a check made of several,
  // each with its own problem.
  repositories?: { repository: string; problem?: string; message: string; fix?: string }[];
}

// WatcherCrash is the Watcher supervisor's report of a crashed Watcher.
export interface WatcherCrash {
  restarts: number;
  lastLine: string;
  restartAt: Date;
}

// START is when a world's clock starts unless told otherwise.
export const START = "2026-11-07T09:00:00Z";

// WORLD_HOST is the machine a world's own service runs on (world.Host).
export const WORLD_HOST = "laptop";

// SERVICE_LIFE_MS is how long a Watcher Lease lasts unless renewed
// (store.ServiceLife).
export const SERVICE_LIFE_MS = 30_000;

// READS are the Api's reads, by name, which actions leaves out.
const READS: ReadonlySet<string> = new Set<keyof Api>([
  "listProjects", "needsMe", "findTickets", "getTicket", "getSpec", "findSpecs", "specQuestions", "options", "transcript", "usage", "sessionUsage", "ciOverview",
]);

// FEEDS are the Api's subscribes, by the stream kind each follows.
const FEEDS = { subscribe: "events", subscribeTranscript: "transcripts", subscribeChat: "chats" } as const;
type Feed = (typeof FEEDS)[keyof typeof FEEDS];
// Subscribe is any of the Api's subscribes: each answers its feed's close.
type Subscribe = (...a: never[]) => () => void;

// Busy is cmd/corewasm's settled report (world.Busy): what keeps the world
// from being settled, with nothing set once it is.
interface Busy {
  requests?: number;
  events?: Streams;
  transcripts?: Streams;
  chats?: Streams;
  unreadWrites?: number;
}

interface Streams {
  open: number;
  subscribed: number;
}

// SETTLE_MS is how long settled waits, by the wall clock, before it gives up.
const SETTLE_MS = 2000;

// wallClock and realSetTimeout are saved as the module loads, so settled
// keeps time and polls for real while a test fakes timers.
const wallClock = Date.now;
const realSetTimeout = globalThis.setTimeout;

// nextTask resolves on the next macrotask, on a real timer even while a
// test fakes timers.
export function nextTask(): Promise<void> {
  return new Promise((r) => realSetTimeout(r));
}

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
  // feeds counts the feeds of each kind subscribed on api, each until it
  // is closed.
  private feeds: Record<Feed, number> = { events: 0, transcripts: 0, chats: 0 };
  // started counts the fetches started so far, and pending those whose
  // core.fetch hasn't settled: Go may see a request only after core.fetch
  // has returned.
  private started = 0;
  private pending = 0;
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
      this.started++;
      this.pending++;
      try {
        return await core.fetch(id, init.method ?? "GET", String(input), [...new Headers(init.headers)], init.body ?? undefined, init.signal ?? undefined);
      } finally {
        this.pending--;
      }
    };
    const api = createHttpApi({ fetch: this.fetch, retryMs: 50 });
    this.api = new Proxy(api, {
      get: (t, k: string) => {
        const v = t[k as keyof Api];
        if (Object.hasOwn(FEEDS, k)) return this.counted(FEEDS[k as keyof typeof FEEDS], v as Subscribe);
        if (typeof v !== "function") return v;
        return (...a: unknown[]) => {
          this.calls.push(k === "act" ? String(a[2]) : k);
          return (v as (...a: unknown[]) => unknown)(...a);
        };
      },
    });
  }

  // subscriptions counts the change feeds open on api now.
  subscriptions(): number {
    return this.feeds.events;
  }

  // counted is subscribe, counting each feed of kind while it is open.
  private counted(kind: Feed, subscribe: Subscribe): Subscribe {
    return (...a) => {
      const close = subscribe(...a);
      this.feeds[kind]++;
      let open = true;
      return () => {
        if (open) this.feeds[kind]--;
        open = false;
        close();
      };
    };
  }

  // fetchesStarted counts the fetches started on the world so far, streams'
  // and the Driver's as well as api's reads and writes.
  fetchesStarted(): number {
    return this.started;
  }

  // settled resolves once the world is settled (ADR 0044): no request in
  // flight, every feed subscribed on api open, and every frame written to
  // an open stream read. It checks at once, then once per macrotask on a
  // real timer, so a test that fakes timers doesn't stall it. After
  // SETTLE_MS it rejects, naming what is still busy.
  async settled(): Promise<void> {
    const deadline = wallClock() + SETTLE_MS;
    for (let busy = this.busy(); busy; busy = this.busy()) {
      if (wallClock() >= deadline) throw new Error(`the world did not settle within ${SETTLE_MS} ms: ${busy}`);
      await nextTask();
    }
  }

  // busy says what keeps the world from being settled, or nothing once it
  // is.
  private busy(): string | undefined {
    const { events, transcripts, chats } = this.feeds;
    const go = JSON.parse(this.live().settled(this.id, events, transcripts, chats)) as Busy;
    const streams = (name: string, s?: Streams) => s && `${name}: ${s.subscribed} subscribed, ${s.open} open`;
    const busy = [
      this.pending > 0 && `fetches not yet answered: ${this.pending}`,
      go.requests && `requests in flight: ${go.requests}`,
      streams("change feeds", go.events),
      streams("Transcript streams", go.transcripts),
      streams("Chat streams", go.chats),
      go.unreadWrites && `streams with a frame not yet read: ${go.unreadWrites}`,
    ].filter(Boolean);
    return busy.length > 0 ? busy.join("; ") : undefined;
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
  // setIssuesCheck for GitHub Issues, setAzureBoardsCheck for Azure
  // Boards and setPullRequestCheck for the Pull Request Landing Method.
  setCICheck(res: CheckResult = {}) {
    this.live().setCICheck(this.id, JSON.stringify(res));
  }

  setIssuesCheck(res: CheckResult = {}) {
    this.live().setIssuesCheck(this.id, JSON.stringify(res));
  }

  setAzureBoardsCheck(res: CheckResult = {}) {
    this.live().setAzureBoardsCheck(this.id, JSON.stringify(res));
  }

  setPullRequestCheck(res: CheckResult = {}) {
    this.live().setPullRequestCheck(this.id, JSON.stringify(res));
  }

  // reportWatcherCrash reports that key's Watcher of kind crashed, as the
  // service's Watcher supervisor does.
  reportWatcherCrash(kind: "ci" | "issues" | "pull_request", key: string, crash: WatcherCrash) {
    this.live().reportWatcherCrash(this.id, kind, key, crash.restarts, crash.lastLine, crash.restartAt.toISOString());
  }

  // holdWatcherLease has the service on host hold key's Watcher Lease, as
  // the service's Watcher supervisor does: the world's own, on
  // WORLD_HOST, or another service on the same store, which takes it if
  // it is free. The world's service then sees where it is held.
  holdWatcherLease(key: string, host: string) {
    this.live().holdWatcherLease(this.id, key, host);
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
