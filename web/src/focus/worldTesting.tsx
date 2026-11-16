// Shared set-up for the Inbox and Focus Mode tests (Spec code-central-580):
// the real App on a WASM Core world, opened at a URL, built with the
// scenario builder, and the helpers every one of them reaches for.
import { screen, within } from "@testing-library/react";
import { newWorld, type World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { scenario, settle } from "../worldTesting";

export { renderApp, scenario } from "../worldTesting";

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// loadedDashboard waits for the Activity Dashboard in main, then for w to
// settle, so a change made next reaches the subscription it makes on
// mounting.
export async function loadedDashboard(w: World, main: HTMLElement) {
  const d = await within(main).findByRole("region", { name: "Activity Dashboard" });
  await settle(w);
  return d;
}

// acme is a world with the Project "acme" ("Acme"), and its scenario,
// which settles inside act after every step.
export async function acme(): Promise<{ w: World; s: Scenario }> {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

// allClearOn sets the Focus Mode Option to All clear, for the tests of All
// clear itself: a fresh store shows the Activity Dashboard.
export async function allClearOn(w: World) {
  await scenario(w).human("/api/set_options", { focus_clear: "all_clear" });
}

// waiting is acme with All clear on and acme-2 waiting on Round 1
// (Questions 1 and 2 open), then acme-3 Failed and acme-4 In Review, in
// that Needs Me order. acme-1 is the Project's Spec.
export async function waiting() {
  const { w, s } = await acme();
  await allClearOn(w);
  const asked = await s.ticket("acme", {
    title: "Pick a colour",
    state: "Needs Answer",
  });
  const failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
  const review = await s.ticket("acme", { title: "Login page", state: "In Review" });
  return { w, s, asked, failed, review };
}
