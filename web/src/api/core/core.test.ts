// The WASM Core's worlds: each a fresh Core, reached through the real
// createHttpApi, with its own clock and the writes it was asked for.
import { describe, expect, it } from "vitest";
import { scenario } from "../scenario";
import { dropWorlds, newWorld } from "./core";

describe("a WASM Core world", () => {
  it("is a fresh Core with an empty store of its own", async () => {
    const a = await newWorld();
    await scenario(a).project("acme", "Acme");
    expect(await a.api.listProjects()).toMatchObject([{ key: "acme", display_name: "Acme" }]);
    expect(await (await newWorld()).api.listProjects()).toEqual([]);
  });

  it("starts its clock where told, and moves it only on advance", async () => {
    const w = await newWorld("2026-03-01T12:00:00Z");
    expect(w.now().toISOString()).toBe("2026-03-01T12:00:00.000Z");
    w.advance(90 * 60_000);
    expect(w.now().toISOString()).toBe("2026-03-01T13:30:00.000Z");
    await scenario(w).project("acme");
    const id = await scenario(w).ticket("acme", { title: "Later" });
    expect((await w.api.getTicket("acme", id)).created).toBe("2026-03-01T13:30:00Z");
  });

  it("records each Api method called, and the writes alone as actions", async () => {
    const w = await newWorld();
    await scenario(w).project("acme");
    await w.api.listProjects();
    await w.api.renameProject("acme", "Acme Web");
    const id = await scenario(w).ticket("acme", { title: "A", state: "Draft" });
    await w.api.act("acme", id, "approve", {});
    expect(w.calls).toEqual(["listProjects", "renameProject", "approve"]);
    expect(w.actions()).toEqual(["renameProject", "approve"]);
  });

  it("ends its live feed when dropped, and fails a later request as an unreachable service", async () => {
    const w = await newWorld();
    let opened!: () => void;
    const open = new Promise<void>((r) => (opened = r));
    const stop = w.api.subscribe({ onEvent: () => {}, onOpen: () => opened() });
    try {
      await open;
      dropWorlds();
      await expect(w.api.listProjects()).rejects.toMatchObject({ code: "service_unavailable" });
      expect(() => w.now()).toThrow(/dropped/);
    } finally {
      stop();
    }
  });
});
