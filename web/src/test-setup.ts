import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";
import { pulseTiming } from "./focus/pulse";

// A pulse lasts as long as a test, so one that checks it never races its end.
pulseTiming.ms = 60_000;
pulseTiming.board = 60_000;

// waitFor and findBy* wait up to 1s by default, which a loaded CI runner
// can use up before a change notice has refetched; a passing test never waits it out.
configure({ asyncUtilTimeout: 5000 });

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
  // Files share their worker's jsdom (isolate: false), so what one stores,
  // such as the Board's column choices, must not reach the next.
  localStorage.clear();
});
