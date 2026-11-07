import { defineConfig } from "vitest/config";

// The real half of the API contract suite (`make e2e`): the real API
// module against a real `code-central serve --store <tmpdir>` started per
// test from CODE_CENTRAL_BIN. Node, not jsdom: it only needs fetch.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.e2e.test.ts"],
    testTimeout: 15000,
  },
});
