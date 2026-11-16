// Shared set-up for the component tests at the UI's root: the real App on a
// WASM Core world (Spec code-central-580), opened at a URL, with the
// world's clock.
import { act, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { App } from "./App";
import type { World } from "./api/core/core";
import { Live } from "./live";

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// renderApp renders the App at url, over w and its clock.
export function renderApp(w: World, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

// SETTLE_ROUNDS is how many rounds of fetching settle waits through before
// it gives up on a UI that keeps fetching.
const SETTLE_ROUNDS = 10;

// Saved as the module loads, so settle polls for real while a test fakes
// timers.
const realSetTimeout = globalThis.setTimeout;

// settle waits, inside act, for w to settle (ADR 0044) and React to render
// what that brought, pass after pass until one starts no fetch: a notice
// makes <Live> refetch, the refetch renders, and the render can fetch or
// subscribe again. Then a test asserts with getBy*. A UI that keeps
// fetching fails it after SETTLE_ROUNDS rounds rather than hang.
export async function settle(w: World) {
  let rounds = 0;
  const round = () => {
    if (++rounds >= SETTLE_ROUNDS) throw new Error(`the UI keeps fetching: it started fetches in ${SETTLE_ROUNDS} rounds of settling`);
  };
  for (;;) {
    const started = w.fetchesStarted();
    await settlePass(w, round);
    if (w.fetchesStarted() === started) return;
    round();
  }
}

// settlePass is one of settle's passes: act around w.settled(), after
// which act renders until React has nothing left to do. A UI that
// refetches as fast as the world answers keeps act rendering, so each poll
// that finds a fetch started since the world settled is a round too.
async function settlePass(w: World, round: () => void) {
  let seen: number | undefined;
  // act answers a thenable whose then answers nothing, not a Promise.
  const done = (async () => {
    await act(async () => {
      await w.settled();
      seen = w.fetchesStarted();
    });
    return true;
  })();
  for (;;) {
    if (await Promise.race([done, new Promise<false>((r) => realSetTimeout(() => r(false)))])) return;
    if (seen !== undefined && w.fetchesStarted() > seen) {
      seen = w.fetchesStarted();
      round();
    }
  }
}

// renderLive renders ui inside <Live> over w, as App mounts it, for a test
// of less than the whole App. Its rerender keeps ui inside <Live> too.
export function renderLive(w: World, ui: ReactNode) {
  return render(ui, { wrapper: ({ children }) => <Live api={w.api}>{children}</Live> });
}

// allClear sets the Focus Mode Option to All clear, for the tests of All
// clear itself: a fresh store shows the Activity Dashboard.
export async function allClear(w: World) {
  await w.api.setOptions({ focus_clear: "all_clear" });
}
