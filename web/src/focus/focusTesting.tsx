// Shared set-up for the Focus Mode tests: the real App against FakeApi,
// opened at a URL, and the helpers every Focus test reaches for.
import { act, render, screen } from "@testing-library/react";
import { App } from "../App";
import type { FakeApi } from "../api/fake";

// renderApp renders the App at url, against fake and its clock.
export function renderApp(fake: FakeApi, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// change reseeds Needs Me and sends the notice the service would.
export function change(fake: FakeApi, groups: Parameters<FakeApi["setNeedsMe"]>[0], id = "acme-1") {
  fake.setNeedsMe(groups);
  act(() => fake.emit({ type: "ticket.changed", project: id.split("-")[0], id }));
}
