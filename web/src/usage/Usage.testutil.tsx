import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "../App";
import { newWorld, nextTask, type World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import type { Api } from "../api/api";
import type { UsageResponse } from "../api/types";
import { scenario, settle } from "../worldTesting";
import { chooseProject, projectPicker } from "../testPickers";

export async function projects() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  await s.project("old", "Old");
  await w.api.archiveProject("old");
  const ticket = await s.ticket("acme", {
    title: "Pick a colour",
    state: "Ready",
  });
  return { w, s, ticket };
}

// calls makes n agent MCP calls of op to project, which Core records as
// Usage with their sizes: a call for a Ticket that isn't there is an error.
export async function calls(s: Scenario, project: string, op: string, n: number, args: Record<string, unknown>) {
  for (let i = 0; i < n; i++) await s.mcpCall(project, op as "get_ticket", args);
}

// world is the world the test last rendered over, which the helpers settle
// after an interaction.
export let world: World;

// renderApp renders the App over w, at url, and settles.
export async function renderApp(w: World, url = "/", api: Api = w.api) {
  world = w;
  window.history.replaceState(null, "", url);
  const r = render(<App api={api} now={w.now} />);
  await settle(w);
  return r;
}

export const usage = () => screen.getByRole("main", { name: "Usage" });
export const table = () => within(usage()).getByRole("table", { name: "Code Central calls" });
// scope is the Project picker's button; chooseScope picks a Project by its shown name.
export const scope = () => projectPicker(usage());
export const chooseScope = (name: string) => chooseProject(usage(), name);
export const range = () => within(usage()).getByRole("combobox", { name: "Range" });
// column is the text of each body row's cell under the header named.
export function column(name: string): string[] {
  const headers = within(table()).getAllByRole("columnheader");
  const i = headers.findIndex((h) => h.textContent?.startsWith(name));
  return within(table())
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole(i === 0 ? "rowheader" : "cell")[i === 0 ? 0 : i - 1].textContent ?? "");
}
export const ops = () => column("Operation");
export const tab = (name: string) => within(usage()).getByRole("tab", { name });
// back is the browser's Back, settled: jsdom fires popstate a task after
// the call.
export const back = async () => {
  await act(async () => {
    window.history.back();
    await nextTask();
  });
  await settle(world);
};
export const openTab = async (name: string) => {
  await userEvent.click(tab(name));
  await settle(world);
};

// claimed has the Factory claim a new Ticket in project for session and
// post the session's model responses, returning the Ticket's ID.
export async function claimed(s: Scenario, project: string, title: string, session: string, usage: UsageResponse[]): Promise<string> {
  const id = await s.ticket(project, { title });
  await s.factorySession(project, id, { session, usage });
  return id;
}

// response is one model response, minutes after the world's now, reading
// context tokens in all.
export function response(w: World, minutes: number, kind: string, context: number, agent?: string): UsageResponse {
  const time = new Date(w.now().getTime() + minutes * 60_000).toISOString();
  return {
    time,
    ...(agent ? { agent } : {}),
    kind,
    input_tokens: 4,
    cache_write_tokens: 1_000,
    cache_read_tokens: context - 1_004,
    output_tokens: 50,
  };
}
