import { beforeEach, expect, it } from "vitest";
import type { ChangeEvent } from "../types";
import { ApiError } from "../types";
import type { Driver } from "../driver";

export function optionsContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- Options (Spec code-central-160) ----

  it("reads the default Options, and sets focus_clear and reads it back", async () => {
    expect(await d.api.options()).toEqual({ focus_clear: "activity_dashboard", focus_popup: "off", limits_popup: "on", gh_path: "" });

    expect(await d.api.setOptions({ focus_clear: "all_clear" })).toEqual({ focus_clear: "all_clear", focus_popup: "off", limits_popup: "on", gh_path: "" });
    expect(await d.api.options()).toEqual({ focus_clear: "all_clear", focus_popup: "off", limits_popup: "on", gh_path: "" });
    expect(await d.api.setOptions({})).toEqual({ focus_clear: "all_clear", focus_popup: "off", limits_popup: "on", gh_path: "" });
  });

  it("sets focus_popup to on, refuses an unknown value, and reads it back", async () => {
    expect((await d.api.options()).focus_popup).toBe("off");
    expect(await d.api.setOptions({ focus_popup: "on" })).toEqual({ focus_clear: "activity_dashboard", focus_popup: "on", limits_popup: "on", gh_path: "" });
    expect((await d.api.options()).focus_popup).toBe("on");

    const bad = await d.api.setOptions({ focus_popup: "loud" } as never).catch((e) => e);
    expect(bad).toMatchObject({ status: 422, code: "invalid", field: "focus_popup" });
    expect(bad.message).toContain("allowed: off, on");
    expect((await d.api.options()).focus_popup).toBe("on");
  });

  it("sets limits_popup to off, refuses an unknown value, and reads it back", async () => {
    expect((await d.api.options()).limits_popup).toBe("on");
    expect(await d.api.setOptions({ limits_popup: "off" })).toEqual({ focus_clear: "activity_dashboard", focus_popup: "off", limits_popup: "off", gh_path: "" });
    expect((await d.api.options()).limits_popup).toBe("off");

    const bad = await d.api.setOptions({ limits_popup: "loud" } as never).catch((e) => e);
    expect(bad).toMatchObject({ status: 422, code: "invalid", field: "limits_popup" });
    expect(bad.message).toContain("allowed: on, off");
    expect((await d.api.options()).limits_popup).toBe("off");
  });

  it("sets the gh path Option to an absolute path, and clears it", async () => {
    const gh = d.absolutePath;
    expect(await d.api.setOptions({ gh_path: ` ${gh} ` })).toEqual({ focus_clear: "activity_dashboard", focus_popup: "off", limits_popup: "on", gh_path: gh });
    expect((await d.api.options()).gh_path).toBe(gh);

    const relative = await d.api.setOptions({ gh_path: "bin/gh" }).catch((e) => e);
    expect(relative).toMatchObject({ status: 422, code: "invalid", field: "gh_path" });
    expect(relative.message).toContain("bin/gh");
    expect((await d.api.options()).gh_path).toBe(gh);

    expect(await d.api.setOptions({ gh_path: "" })).toEqual({ focus_clear: "activity_dashboard", focus_popup: "off", limits_popup: "on", gh_path: "" });
  });

  it("refuses an unknown Option and a bad value, changing nothing", async () => {
    const unknown = await d.api.setOptions({ colour: "red" } as never).catch((e) => e);
    expect(unknown).toBeInstanceOf(ApiError);
    expect(unknown).toMatchObject({ status: 422, code: "invalid", field: "colour" });
    expect(unknown.message).toContain("focus_clear");

    const bad = await d.api.setOptions({ focus_clear: "fireworks" } as never).catch((e) => e);
    expect(bad).toMatchObject({ status: 422, code: "invalid", field: "focus_clear" });
    expect(bad.message).toContain("allowed: activity_dashboard, all_clear");

    expect(await d.api.options()).toEqual({ focus_clear: "activity_dashboard", focus_popup: "off", limits_popup: "on", gh_path: "" });
  });

  it("sends options.changed over the live feed when the Options change", async () => {
    const got: ChangeEvent[] = [];
    let opened!: () => void;
    const open = new Promise<void>((r) => (opened = r));
    const stop = d.api.subscribe({ onEvent: (e) => got.push(e), onOpen: () => opened() });
    try {
      await open;
      await d.api.setOptions({ focus_clear: "all_clear" });
      await expect.poll(() => got, { timeout: 5000 }).toEqual([{ type: "options.changed" }]);
    } finally {
      stop();
    }
  });

  it("delivers a change notice for each write over the live feed", async () => {
    const got: ChangeEvent[] = [];
    let opened!: () => void;
    const open = new Promise<void>((r) => (opened = r));
    const stop = d.api.subscribe({ onEvent: (e) => got.push(e), onOpen: () => opened() });
    try {
      await open;
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "A", "Draft");
      await d.api.act("acme", id, "move", { to_state: "Ready" });

      await expect.poll(() => got.filter((e) => e.type === "ticket.changed").length, { timeout: 5000 }).toBe(2);
      expect(got[0]).toEqual({ type: "project.changed", project: "acme" });
      expect(got.filter((e) => e.type === "ticket.changed")).toEqual([
        { type: "ticket.changed", project: "acme", id },
        { type: "ticket.changed", project: "acme", id },
      ]);
    } finally {
      stop();
    }
  });
}
