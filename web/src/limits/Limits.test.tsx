// The Limits popup (Spec code-central-861): the real App on the WASM Core,
// driven by Heartbeats that carry Plan Limits. It checks, by role, what the
// human sees: a meter per window with its countdown, when the popup shows
// at all, its colours, and how the UI's clock moves it on.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { World } from "../api/core/core";
import { newWorld } from "../api/core/core";
import type { PlanLimits } from "../api/types";
import { renderApp, scenario, settle } from "../worldTesting";

const beat = { factory: "code-central-factory", host: "laptop", state: "online", interval_seconds: 30 } as const;
const MINUTE = 60_000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

// at is the time ms after the world's now, as RFC 3339.
const at = (w: World, ms: number) => new Date(w.now().getTime() + ms).toISOString();

// limits are Plan Limits observed now: 42.4% of the 5-hour window, resetting
// in 2h 14m, and 7% of the weekly one, resetting in 3d 4h.
function limits(w: World, over: Partial<PlanLimits> = {}): PlanLimits {
  return {
    observed_at: at(w, 0),
    five_hour: { used_percent: 42.4, resets_at: at(w, 2 * HOUR + 14 * MINUTE) },
    weekly: { used_percent: 7, resets_at: at(w, 3 * DAY + 4 * HOUR) },
    ...over,
  };
}

// world is a WASM Core with the Project acme, whose Factory reports
// plan_limits, if given.
async function world(plan_limits?: (w: World) => PlanLimits) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (plan_limits) await s.reportHeartbeat("acme", { ...beat, plan_limits: plan_limits(w) });
  return { w, s };
}

const popup = () => screen.getByRole("region", { name: "Limits" });
const meter = (name: string) => within(popup()).getByRole("meter", { name });

describe("the Limits popup", () => {
  it("shows a meter for each window, 5-hour then Weekly, with its percentage and countdown", async () => {
    const { w } = await world(limits);
    await renderApp(w, "/?view=inbox&project=acme");

    const meters = within(popup()).getAllByRole("meter");
    expect(meters.map((m) => m.getAttribute("aria-label"))).toEqual(["5-hour", "Weekly"]);
    expect(meter("5-hour")).toHaveAttribute("aria-valuenow", "42");
    expect(meter("Weekly")).toHaveAttribute("aria-valuenow", "7");
    expect(popup()).toHaveTextContent("5-hour42%resets in 2h 14m");
    expect(popup()).toHaveTextContent("Weekly7%resets in 3d 4h");
    expect(within(popup()).queryByRole("button")).toBeNull();
  });

  it("shows for the Sole Project with no Project in the address", async () => {
    const { w } = await world(limits);
    await renderApp(w, "/");
    expect(meter("5-hour")).toHaveAttribute("aria-valuenow", "42");
  });

  it("shows one meter when only one window is reported", async () => {
    const { w } = await world((w) => limits(w, { five_hour: undefined }));
    await renderApp(w, "/?project=acme");
    expect(within(popup()).getAllByRole("meter").map((m) => m.getAttribute("aria-label"))).toEqual(["Weekly"]);
  });

  it("shows nothing under All Projects", async () => {
    const { w, s } = await world(limits);
    await s.project("beta", "Beta");
    await renderApp(w, "/");
    expect(screen.queryByRole("region", { name: "Limits" })).toBeNull();
  });

  it("shows nothing with the Option off", async () => {
    const { w } = await world(limits);
    await w.api.setOptions({ limits_popup: "off" });
    await renderApp(w, "/?project=acme");
    expect(screen.queryByRole("region", { name: "Limits" })).toBeNull();
  });

  it("shows nothing before any figures, and appears with the first", async () => {
    const { w, s } = await world();
    await s.reportHeartbeat("acme", beat);
    await renderApp(w, "/?project=acme");
    expect(screen.queryByRole("region", { name: "Limits" })).toBeNull();

    await s.reportHeartbeat("acme", { ...beat, plan_limits: limits(w) });
    expect(meter("Weekly")).toHaveAttribute("aria-valuenow", "7");
  });

  it("hides once the Factory goes Offline", async () => {
    const { w, s } = await world(limits);
    await renderApp(w, "/?project=acme");
    popup();

    await s.reportHeartbeat("acme", { ...beat, state: "stopping", note: "stopped: drained", plan_limits: limits(w) });
    expect(screen.queryByRole("region", { name: "Limits" })).toBeNull();
  });

  it.each([
    [74.4, "neither"],
    [75, "amber"],
    [89.4, "amber"],
    [90, "red"],
    [100, "red"],
  ])("colours a window %s per cent used %s", async (used_percent, colour) => {
    const { w } = await world((w) => limits(w, { five_hour: { used_percent, resets_at: at(w, HOUR) } }));
    await renderApp(w, "/?project=acme");
    for (const c of ["amber", "red"]) {
      if (c === colour) expect(meter("5-hour")).toHaveClass(c);
      else expect(meter("5-hour")).not.toHaveClass(c);
    }
  });

  it("hides at once when the Option is turned off in the Options view, and comes back when turned on", async () => {
    const { w } = await world(limits);
    await renderApp(w, "/?view=options&project=acme");
    popup();
    const group = within(screen.getByRole("main", { name: "Options" })).getByRole("radiogroup", { name: "Show Limits popup" });

    await userEvent.click(within(group).getByRole("radio", { name: "Off" }));
    await settle(w);
    expect(screen.queryByRole("region", { name: "Limits" })).toBeNull();

    await userEvent.click(within(group).getByRole("radio", { name: "On" }));
    await settle(w);
    popup();
  });
});

describe("the Limits popup on the UI's clock", () => {
  beforeEach(() => vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] }));
  afterEach(() => vi.useRealTimers());

  // later moves the world's clock and the UI's timers on by ms, with no
  // write, so nothing refetches.
  function later(w: World, ms: number) {
    w.advance(ms);
    act(() => vi.advanceTimersByTime(ms));
  }

  it("moves the countdown on each minute", async () => {
    const { w } = await world(limits);
    await renderApp(w, "/?project=acme");
    expect(popup()).toHaveTextContent("resets in 2h 14m");

    later(w, MINUTE);
    expect(popup()).toHaveTextContent("resets in 2h 13m");
    expect(popup()).toHaveTextContent("resets in 3d 3h");
  });

  it("shows 0% and reset once a window's reset time passes", async () => {
    const { w } = await world((w) => limits(w, { five_hour: { used_percent: 95, resets_at: at(w, 30_000) } }));
    await renderApp(w, "/?project=acme");
    expect(popup()).toHaveTextContent("5-hour95%resets in 1m");

    later(w, MINUTE);
    expect(meter("5-hour")).toHaveAttribute("aria-valuenow", "0");
    expect(meter("5-hour")).not.toHaveClass("red");
    expect(popup()).toHaveTextContent("5-hour0%reset");
    expect(popup()).not.toHaveTextContent("resets in 1m");
  });

  it("notes how old the figures are once they are over 10 minutes old", async () => {
    const { w } = await world((w) => limits(w, { observed_at: at(w, -10 * MINUTE) }));
    await renderApp(w, "/?project=acme");
    expect(popup()).not.toHaveTextContent("updated");

    later(w, MINUTE);
    expect(popup()).toHaveTextContent("updated 11m ago");
  });

  it("notes figures a Factory last read 25 minutes ago", async () => {
    const { w } = await world((w) => limits(w, { observed_at: at(w, -25 * MINUTE) }));
    await renderApp(w, "/?project=acme");
    expect(popup()).toHaveTextContent("updated 25m ago");
  });
});
