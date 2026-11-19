import { beforeEach, expect, it } from "vitest";
import { ApiError } from "../types";
import type { Driver } from "../driver";

export function storageContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- The Storage tab (Spec code-central-1032) ----

  it("reads the Store's size, each Project's counts or one Project's alone, the memory and the services", async () => {
    // createProject makes each Project's one Spec.
    await d.createProject("acme", "Acme");
    await d.createProject("beta", "Beta");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    await d.createTicket("acme", "Pick a font", "Draft");
    await d.mcpCall("acme", "get_ticket", { ticket: id });

    const all = await d.api.storage();
    expect(all.projects).toEqual([
      { key: "acme", name: "Acme", specs: 1, tickets: 2, usage_days: 1 },
      { key: "beta", name: "Beta", specs: 1, tickets: 0, usage_days: 0 },
    ]);
    expect(all.size).toBeGreaterThan(0);
    expect(all.memory.heap).toBeGreaterThan(0);
    expect(all.services).toHaveLength(1);

    const beta = await d.api.storage("beta");
    expect(beta.projects).toEqual([all.projects[1]]);
    expect(beta.services).toHaveLength(1);

    const unknown = await d.api.storage("nope").catch((e) => e);
    expect(unknown).toBeInstanceOf(ApiError);
    expect(unknown).toMatchObject({ status: 404, code: "not_found" });
  });
}
