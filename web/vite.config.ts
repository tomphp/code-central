/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// `npm run dev` serves the UI with hot reload and proxies /api (the HTTP
// API and the SSE feed) to a running `code-central serve`. Point it at
// another service with CODE_CENTRAL_URL, such as a `serve --store <tmp>`
// started with CODE_CENTRAL_LISTEN=127.0.0.1:<port>.
const service = process.env.CODE_CENTRAL_URL ?? "http://127.0.0.1:7733";

export default defineConfig({
  plugins: [react()],
  // web/dist is what `go build -tags webdist` embeds; it is never committed.
  build: { outDir: "dist", emptyOutDir: true },
  server: {
    port: 5173,
    proxy: { "/api": { target: service } },
  },
  // The fast suite (`make test`): component, layout and the fake half of
  // the API contract suite, in jsdom. The real half is *.e2e.test.ts, run
  // by `make e2e` through vitest.e2e.config.ts.
  test: {
    environment: "jsdom",
    include: ["src/**/*.test.{ts,tsx}"],
    exclude: ["src/**/*.e2e.test.ts"],
    setupFiles: ["src/test-setup.ts"],
    // Test files share their workers' module state rather than each
    // getting a fresh environment, which halves the suite's CPU. No file
    // mocks modules or globals, and the setup cleans up after each test,
    // so they don't lean on each other: a shuffled order and one worker
    // pass the same.
    isolate: false,
  },
});
