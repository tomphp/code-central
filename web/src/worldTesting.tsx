// Shared set-up for the component tests at the UI's root: the real App on a
// WASM Core world (Spec code-central-580), opened at a URL, with the
// world's clock.
import { act, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { App } from "./App";
import { nextTask, type World } from "./api/core/core";
import { scenario as worldScenario, type Scenario } from "./api/scenario";
import { Live } from "./live";

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// renderApp renders the App at url, over w and its clock, and settles, so
// a test asserts with getBy* straight after.
export async function renderApp(w: World, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  const r = render(<App api={w.api} now={w.now} />);
  await settle(w);
  return r;
}

// scenario is the scenario builder on w for a component test: each step
// ends with settle(w), inside act, rather than the world's bare settled.
export function scenario(w: World): Scenario {
  return worldScenario(w, () => settle(w));
}

// SETTLE_ROUNDS is how many rounds of fetching settle waits through before
// it gives up on a UI that keeps fetching.
const SETTLE_ROUNDS = 10;

// settle waits, inside act, for w to settle (ADR 0044) and React to render
// what that brought, pass after pass until one starts no fetch: a notice
// makes <Live> refetch, the refetch renders, and the render can fetch or
// subscribe again. Then a test asserts with getBy*. A UI that keeps
// fetching fails it after SETTLE_ROUNDS rounds rather than hang.
export async function settle(w: World) {
  let rounds = 0;
  const fetched = () => {
    if (++rounds > SETTLE_ROUNDS) throw new Error(`the UI keeps fetching: it was still starting fetches after ${SETTLE_ROUNDS} rounds of settling`);
  };
  // A pass that starts no fetch isn't the end on a loaded machine: the
  // world counts a response answered before the UI has read its body and
  // rendered it, and what the render starts comes a task or two later. So
  // the UI must stay quiet through QUIET_PASSES passes, each after a
  // macrotask, before settle returns.
  for (let quiet = 0; quiet < QUIET_PASSES; ) {
    const started = w.fetchesStarted();
    await settlePass(w, fetched);
    quiet = w.fetchesStarted() === started ? quiet + 1 : 0;
    if (quiet < QUIET_PASSES) await nextTask();
  }
}

// QUIET_PASSES is how many passes in a row settle sees start no fetch
// before it returns.
const QUIET_PASSES = 3;

// settlePass is one of settle's passes: act around w.settled(), after
// which act renders until React has nothing left to do. It calls fetched
// for each round of fetching it sees once the world has settled: each
// poll that finds a fetch started since the last, and the end of the pass
// if it does. A UI that refetches as fast as the world answers keeps act
// rendering, so fetched's limit, not the pass's end, stops it.
async function settlePass(w: World, fetched: () => void) {
  let seen = w.fetchesStarted();
  let settled = false;
  // act answers a thenable whose then answers nothing, not a Promise.
  const done = (async () => {
    await act(async () => {
      await w.settled();
      settled = true;
    });
    return true;
  })();
  for (;;) {
    const ended = await Promise.race([done, nextTask().then(() => false)]);
    if (settled && w.fetchesStarted() > seen) {
      seen = w.fetchesStarted();
      fetched();
    }
    if (ended) return;
  }
}

// renderLive renders ui inside <Live> over w, as App mounts it, for a test
// of less than the whole App, and settles. Its rerender keeps ui inside
// <Live> too.
export async function renderLive(w: World, ui: ReactNode) {
  const r = render(ui, { wrapper: ({ children }) => <Live api={w.api}>{children}</Live> });
  await settle(w);
  return r;
}

// allClear sets the Focus Mode Option to All clear, for the tests of All
// clear itself: a fresh store shows the Activity Dashboard.
export async function allClear(w: World) {
  await w.api.setOptions({ focus_clear: "all_clear" });
}
