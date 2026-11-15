import { beforeEach, expect, it } from "vitest";
import type { Driver } from "../driver";
import { defaultCI, defaultLanding } from "../scenario";

export function projectsContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  it("lists every Project with its Needs Me count", async () => {
    await d.createProject("beta", "Beta");
    await d.createProject("acme", "Acme Corp");
    const id = await d.createTicket("acme", "Pick a colour", "Ready");
    await d.postRound("acme", id, [{ title: "Which colour?", body: "Red or blue", recommended: "Blue" }]);

    const rows = await d.api.listProjects();
    expect(rows.map((r) => [r.key, r.display_name, r.needs_me, r.archived])).toEqual([
      ["acme", "Acme Corp", 1, false],
      ["beta", "Beta", 0, false],
    ]);
    expect(rows.map((r) => r.landing)).toEqual([defaultLanding(), defaultLanding()]);
    expect(rows.map((r) => r.ci)).toEqual([defaultCI(), defaultCI()]);
  });
}
