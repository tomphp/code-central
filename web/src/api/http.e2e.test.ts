// The serve half of the API contract suite (`make e2e`): createHttpApi
// against a real `code-central serve --store <tmpdir>`, a fresh one per
// test, from the already-built binary in CODE_CENTRAL_BIN, driven by the
// same Driver as the WASM half (fake.test.ts). Outside Core it has the
// fake gh and real connections. It never binds port 7733.
import { serve } from "../e2e/serve";
import { apiContract } from "./contract/index";
import { HttpDriver } from "./driver";
import { createHttpApi } from "./http";

apiContract("HTTP API against code-central serve", async () => {
  const svc = await serve();
  // Transcript streams go through a fetch the driver can cut, as a flaky
  // connection would (ticket 110).
  const streams = new Set<AbortController>();
  const cuttable: typeof fetch = (input, init) => {
    const url = String(input);
    if (!url.endsWith("/transcript/stream") && !url.endsWith("/chat/stream")) return fetch(input, init);
    const cut = new AbortController();
    streams.add(cut);
    init?.signal?.addEventListener("abort", () => cut.abort());
    return fetch(input, { ...init, signal: cut.signal });
  };
  return new HttpDriver({
    url: svc.url,
    fetch,
    api: createHttpApi({ baseUrl: svc.url, retryMs: 50, fetch: cuttable }),
    logOutOfGH: svc.logOutOfGH,
    letGHTriage: svc.letGHTriage,
    letGHWrite: svc.letGHWrite,
    dropTranscriptStreams: () => {
      for (const cut of streams) cut.abort();
      streams.clear();
    },
    // The real service's Azure Boards check needs a PAT and Azure DevOps.
    canReachAzureDevOps: false,
    // Absolute where the service runs: a Windows path isn't on Unix.
    absolutePath: process.platform === "win32" ? "C:\\Tools\\gh.exe" : "/opt/tools/gh",
    // serve has nothing to settle on (ADR 0044): its contract cases poll.
    settle: async () => {},
    close: svc.stop,
  });
});
