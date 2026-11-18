import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect } from "vitest";
import { App } from "../App";
import type { Api } from "../api/api";
import { newWorld, nextTask, type World } from "../api/core/core";
import { type Scenario } from "../api/scenario";
import { scenario, settle } from "../worldTesting";

export const MINUTE = 60_000;

// Fake is a world, its scenario builder and the IDs of what world made.
export interface Fake {
  w: World;
  s: Scenario;
  ids: { spec: string; form: string; signup: string; old: string; dropped: string; reset: string; beta: string };
}

// world makes acme with a Spec (acme-1) and Tickets in several States,
// created a minute apart, and beta with one Ticket.
export async function world(): Promise<Fake> {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const spec = await s.spec("acme", { title: "Login" }); // acme-1
  const next = () => w.advance(MINUTE);
  next();
  const form = await s.ticket("acme", { title: "Login form", state: "Ready", spec }); // acme-2
  next();
  const signup = await s.ticket("acme", { title: "Sign-up page", state: "In Progress", spec }); // acme-3
  next();
  const old = await s.ticket("acme", { title: "Old work", state: "Done", spec }); // acme-4
  next();
  const dropped = await s.ticket("acme", { title: "Dropped", state: "Cancelled", spec }); // acme-5
  next();
  const reset = await s.ticket("acme", { title: "Password reset", state: "Ready", spec, labels: ["prototype"] }); // acme-6
  await s.addBlocker("acme", reset, signup);
  next();
  const beta = await s.ticket("beta", { title: "Beta export", state: "In Progress" });
  next();
  return { w, s, ids: { spec, form, signup, old, dropped, reset, beta } };
}

// renderApp renders the App at url on f's world, with over's Api methods in
// place of the world's.
export async function renderApp(f: Fake, url = "/", over: Partial<Api> = {}) {
  rendered = f.w;
  window.history.replaceState(null, "", url);
  const api = new Proxy(f.w.api, { get: (t, k: string) => (k in over ? over[k as keyof Api] : t[k as keyof Api]) });
  const r = render(<App api={api} now={f.w.now} />);
  await settle(f.w);
  return r;
}

// rendered is the world renderApp last rendered over; settled waits for it
// after a user-event interaction or a direct call on its Api.
export let rendered: World;
export const settled = () => settle(rendered);
// click is a user's click, settled.
export const click = async (el: Element) => {
  await userEvent.click(el);
  await settled();
};
// travel is the browser's Back or Forward, settled: jsdom fires popstate a
// task after the call.
export const travel = async (go: () => void) => {
  await act(async () => {
    go();
    await nextTask();
  });
  await settled();
};
// press is a key press, settled.
export const press = async (keys: string) => {
  await userEvent.keyboard(keys);
  await settled();
};

// needing has Sign-up page (In Progress) asking a Round and Login form
// (Ready) In Review: both need the human, in Needs Me's order.
export async function needing(f: Fake) {
  await f.s.askRound("acme", f.ids.signup);
  await f.s.inReview("acme", f.ids.form);
}

export const nav = () => screen.getByRole("navigation", { name: "Filters" });
export const boardLink = () => within(nav()).getByRole("link", { name: /^Board/ });
export const board = () => screen.getByRole("main", { name: "Board" });
export const heads = () => within(board()).getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
export const column = (state: string) => within(board()).getByRole("region", { name: state });
export const cards = (state: string) => within(column(state)).queryAllByRole("link");
export const titles = (state: string) => cards(state).map((c) => c.querySelector(".board-card-title")?.textContent);
export const card = (title: string) => within(board()).getByRole("link", { name: new RegExp(title) });
export const columnsButton = () => within(board()).getByRole("button", { name: /^Columns/ });
export const loaded = () => within(board()).getByRole("link", { name: /Login form/ });
// noPopup settles the world, then checks no popup, nor any other dialog, is
// open: a popup closes as the world's last change reaches it.
export const noPopup = async () => {
  await settled();
  expect(screen.queryByRole("dialog")).toBeNull();
};
