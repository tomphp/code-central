import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";
import { dropWorlds } from "./api/core/core";
import { pulseTiming } from "./focus/pulse";

// A test never reaches the human's real service on 7733: fetch throws on
// it, so a test that builds an API against the default fails at once.
const realFetch = globalThis.fetch;
globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
  const raw = input instanceof Request ? input.url : String(input);
  if (new URL(raw, window.location.href).port === "7733") {
    throw new Error(`a test fetched ${raw}, the real Code Central service; use a WASM Core world or a test service on another port`);
  }
  return realFetch(input, init);
}) as typeof fetch;

// A pulse lasts as long as a test, so one that checks it never races its end.
pulseTiming.ms = 60_000;
pulseTiming.board = 60_000;

// waitFor and findBy* wait up to 1s by default, which a loaded CI runner
// can use up before a change notice has refetched; a passing test never waits it out.
configure({ asyncUtilTimeout: 5000 });

afterEach(() => {
  cleanup();
  // Each test's WASM Core worlds go with it, so a worker's memory doesn't
  // grow with every test.
  dropWorlds();
  window.history.replaceState(null, "", "/");
  // Files share their worker's jsdom (isolate: false), so what one stores,
  // such as the Board's column choices, must not reach the next.
  localStorage.clear();
});
