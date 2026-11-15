// Shared set-up for the component tests at the UI's root: the real App on a
// WASM Core world (Spec code-central-580), opened at a URL, with the
// world's clock.
import { render, screen } from "@testing-library/react";
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
