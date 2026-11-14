// Shared set-up for the component tests at the UI's root: the real App on a
// WASM Core world (Spec code-central-580), opened at a URL, with the
// world's clock.
import { render, screen } from "@testing-library/react";
import { App } from "./App";
import type { World } from "./api/core/core";

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// renderApp renders the App at url, over w and its clock.
export function renderApp(w: World, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

// allClear sets the Focus Mode Option to All clear, for the tests of All
// clear itself: a fresh store shows the Activity Dashboard.
export async function allClear(w: World) {
  await w.api.setOptions({ focus_clear: "all_clear" });
}
