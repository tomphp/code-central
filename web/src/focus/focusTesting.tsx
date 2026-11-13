// Shared set-up for the Focus Mode tests: the real App against FakeApi,
// opened at a URL, and the helpers every Focus test reaches for.
import { act, render, screen, within } from "@testing-library/react";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { RoundView } from "../api/types";

// renderApp renders the App at url, against fake and its clock.
export function renderApp(fake: FakeApi, url = "/?view=focus") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

// focus is Focus Mode's main region.
export const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// without is fake's Needs Me with the Ticket id taken out.
export function without(fake: FakeApi, id: string) {
  return fake.needsMeGroups.map((g) => ({ ...g, tickets: g.tickets.filter((r) => r.id !== id) })).filter((g) => g.tickets.length);
}

// change reseeds Needs Me and sends the notice the service would.
export function change(fake: FakeApi, groups: Parameters<FakeApi["setNeedsMe"]>[0], id = "acme-1") {
  fake.setNeedsMe(groups);
  act(() => fake.emit({ type: "ticket.changed", project: id.split("-")[0], id }));
}

// allClearOn sets the Focus Mode Option to All clear, for the tests of All
// clear itself: a fresh store shows the Activity Dashboard.
export function allClearOn(fake: FakeApi) {
  fake.seedOptions({ focus_clear: "all_clear" });
}

// waiting has acme-1 waiting on Round 1 (Questions 1 and 2 open), then
// acme-2 Failed and acme-3 In Review, in that Needs Me order, with All
// clear on.
export function waiting() {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  const rounds: RoundView[] = [
    {
      number: 1,
      claim: 1,
      posted: fake.now.toISOString(),
      questions: [
        { number: 1, title: "Which colour?", body: "Red or blue", recommended: "Blue", status: "open" },
        { number: 2, title: "Which size?", body: "S or M", status: "open" },
      ],
    },
  ];
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer", rounds }); // acme-1
  fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-2
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-3
  fake.setNeedsMe([
    { reason: "needs_answer", tickets: [{ id: "acme-1", round: { number: 1, open: 2, questions: 2 } }] },
    { reason: "failed", tickets: [{ id: "acme-2" }] },
    { reason: "in_review", tickets: [{ id: "acme-3", summary: "Added the login page" }] },
  ]);
  return fake;
}

// waitingTickets is waiting without its Round, so every Focus Item is a
// Ticket's.
export function waitingTickets() {
  const fake = waiting();
  fake.setNeedsMe(fake.needsMeGroups.filter((g) => g.reason !== "needs_answer"));
  return fake;
}

// loadedDashboard waits for the Activity Dashboard in main, then for its
// effects to run, so a notice sent next reaches the subscription it makes on
// mounting.
export async function loadedDashboard(main: HTMLElement) {
  const d = await within(main).findByRole("region", { name: "Activity Dashboard" });
  await act(async () => {});
  return d;
}
