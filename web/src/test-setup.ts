import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";
import { pulseTiming } from "./focus/pulse";

// A pulse lasts as long as a test, so one that checks it never races its end.
pulseTiming.ms = 60_000;

afterEach(() => {
  cleanup();
  window.history.replaceState(null, "", "/");
});
