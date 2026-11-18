import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "../App";
import type { Api } from "../api/api";
import { newWorld, type World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import type { SubmitRequirementResult } from "../api/types";
import { scenario, settle } from "../worldTesting";

export const minutes = (m: number) => m * 60_000;

// startWorld is a fresh world with Projects keys, and its scenario.
export async function startWorld(...keys: string[]) {
  const w = await newWorld();
  const s = scenario(w);
  for (const k of keys) await s.project(k, k[0].toUpperCase() + k.slice(1));
  // A lone Project would be the Sole Project; these tests are of the All Projects scope.
  if (keys.length === 1) await s.project("spare", "Spare");
  return { w, s };
}

// requirement submits a Requirement as the human: its Spec and Stage Tickets.
export async function requirement(s: Scenario, key: string, text: string): Promise<SubmitRequirementResult> {
  return s.human(`${s.projectPath(key)}/submit_requirement`, { text });
}

// done takes a Ready Ticket to Done by hand.
export async function done(s: Scenario, key: string, id: string) {
  await s.human(`${s.ticketPath(key, id)}/move`, { to_state: "Done" });
}

// doneStages takes every Stage Ticket of a Requirement to Done.
export async function doneStages(s: Scenario, key: string, r: SubmitRequirementResult) {
  for (const t of r.stages) await done(s, key, t.id);
}

// world is Projects acme and beta. acme: acme-1 is its Spec, acme-2 Needs
// Answer (Round 1 with one of two Questions open, 5m ago), acme-3 Failed
// twice (90m ago), acme-4 a Draft, acme-5 Ready. beta: beta-1 is its Spec,
// beta-2 In Review (3h ago), beta-3 Ready, beta-4 a Draft.
export async function world() {
  const { w, s } = await startWorld("acme", "beta");
  await s.ticket("acme", { title: "Pick a colour" }); // acme-2
  await s.ticket("acme", { title: "Fix login" }); // acme-3
  await s.ticket("acme", { title: "Rough idea", state: "Draft" }); // acme-4
  await s.ticket("acme", { title: "Acme ready" }); // acme-5
  await s.ticket("beta", { title: "Beta review" }); // beta-2
  await s.ticket("beta", { title: "Beta ready" }); // beta-3
  await s.ticket("beta", { title: "Beta sketch", state: "Draft" }); // beta-4
  await s.inReview("beta", "beta-2", undefined, "Added the login page");
  w.advance(minutes(90));
  await s.fail("acme", "acme-3", { ...REPORT_OF, what_broke: "Tests fail on CI" });
  await s.human(`${s.ticketPath("acme", "acme-3")}/send_back`, {});
  await s.fail("acme", "acme-3", { ...REPORT_OF, what_broke: "Tests fail on CI" });
  w.advance(minutes(85));
  await s.askRound("acme", "acme-2", [
    { title: "Which colour?", body: "Red or blue" },
    { title: "Which size?", body: "S or M" },
  ]);
  await s.human(`${s.ticketPath("acme", "acme-2")}/answer`, { question: 1, text: "Blue" });
  w.advance(minutes(5));
  return w;
}

export const REPORT_OF = { what_broke: "", why: "The fixture server never starts", what_to_fix: "Start the fixture in TestMain", where_work_stands: "Branch impl, tests red" };

// renderApp renders the App over api (the world's own unless a test injects
// one) and settles, so a test asserts with getBy* straight after.
export async function renderApp(w: World, url = "/", api: Api = w.api) {
  window.history.replaceState(null, "", url);
  const r = render(<App api={api} now={w.now} />);
  await settle(w);
  return r;
}

// cancelAll cancels the Tickets, as the human does to clear them from Needs Me.
export async function cancelAll(w: World, key: string, ids: string[]) {
  const s = scenario(w);
  for (const id of ids) await s.human(`${s.ticketPath(key, id)}/cancel`, {});
}

export const list = () => screen.getByRole("main", { name: "Tickets" });
export const nav = () => screen.getByRole("navigation", { name: "Filters" });
export const detail = () => screen.getByRole("complementary", { name: "Ticket" });
export const groupNames = () => within(list()).queryAllByRole("heading", { level: 3 }).map((h) => h.dataset.reason);
// Every row in the middle pane, collapsed Drafts included, in DOM order.
export const rowIds = () => [...list().querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
export const params = () => new URLSearchParams(window.location.search);


// requirements is a Project with three Requirements and a plain Ticket:
// acme-1 at Spec, which needs an answer; acme-5 in Build, with two
// implementation Tickets needing the human; acme-11 waiting for a Factory
// to claim Grilling. acme-15 is a plain Spec, with acme-16 Failed on it.
export async function requirements() {
  const { w, s } = await startWorld("acme");
  const first = await requirement(s, "acme", "Export a Spec as Markdown"); // acme-1, stages acme-2..4
  await done(s, "acme", first.stages[0].id);
  await s.askRound("acme", first.stages[1].id, [{ title: "Which format?", body: "CommonMark or GFM" }]);
  const building = await requirement(s, "acme", "Archive finished Projects"); // acme-5, stages acme-6..8
  await doneStages(s, "acme", building);
  const spec = building.spec.id;
  const review = await s.ticket("acme", { title: "Archive in the Project menu", spec }); // acme-9
  const failed = await s.ticket("acme", { title: "CLI: archive-project", spec }); // acme-10
  await requirement(s, "acme", "Render Markdown"); // acme-11, stages acme-12..14
  const plain = await s.spec("acme", { title: "Plain" }); // acme-15
  const lint = await s.ticket("acme", { title: "Plain work", spec: plain }); // acme-16
  await s.inReview("acme", review, undefined, "Menu item added");
  await s.fail("acme", failed, { ...REPORT_OF, what_broke: "Tests fail" });
  await s.fail("acme", lint, { ...REPORT_OF, what_broke: "Lint" });
  return w;
}

export const groupHeads = () => within(list()).queryAllByRole("heading", { level: 3 }).map((h) => h.textContent);

// bySpecWorld seeds Tickets on a Requirement (acme-1, with its Stages
// acme-2..4), a plain Spec and a Map, across States.
export async function bySpecWorld() {
  const { w, s } = await startWorld("acme", "beta");
  const req = await requirement(s, "acme", "Export a Spec as Markdown"); // acme-1, Stages acme-2..4 (Ready)
  await done(s, "acme", req.stages[0].id);
  const plain = await s.spec("acme", { title: "Archive finished Projects" }); // acme-5
  await s.ticket("acme", { title: "Archive menu", spec: plain }); // acme-6
  const cli = await s.ticket("acme", { title: "Archive CLI", spec: plain }); // acme-7
  await s.ticket("acme", { title: "Archive docs", spec: plain }); // acme-8
  await s.human(`${s.projectPath("acme")}/submit_map`, { text: "Wayfinding" }); // acme-9, Charting Ticket acme-10 (Ready)
  await s.ticket("acme", { title: "Export button", spec: req.spec.id }); // acme-11
  const other = await s.spec("beta", { title: "Beta plan" }); // beta-1
  await s.ticket("beta", { title: "Beta thing", spec: other }); // beta-2
  await s.fail("acme", cli, { ...REPORT_OF, what_broke: "Tests fail" });
  return w;
}

export const specRows = () => within(list()).queryAllByRole("region");
export const specRow = (name: string) => within(list()).getByRole("region", { name });
// The Tickets an open Spec row lists; none while it is closed.
export const openIds = (name: string) => {
  const tickets = within(specRow(name)).queryByRole("group", { name: `Tickets of ${name}` });
  return tickets ? within(tickets).getAllByRole("link").map((a) => a.dataset.id) : [];
};
// The Spec row's own link, the one that opens the Spec.
export const specLinkOf = (name: string) => specRow(name).querySelector<HTMLElement>(`a[data-id]`)!;
export const bySpec = () => userEvent.click(within(list()).getByRole("button", { name: "Spec" }));
