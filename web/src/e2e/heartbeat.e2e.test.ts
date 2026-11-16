// End-to-end (`make e2e`): the Heartbeat request docs/writing-a-factory.md
// shows, sent with fetch to a real service, then a stopping Heartbeat, with
// the Project's factory_status read back.
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the documented Heartbeat", () => {
  let svc: Service;
  beforeAll(async () => {
    svc = await serve();
  }, 30_000);
  afterAll(async () => {
    await svc?.stop();
  });

  it("an online Heartbeat shows the Factory Online, a stopping one Offline with its note", async () => {
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const heartbeat = (state: string, note: string) =>
      fetch(`${svc.url}/api/p/acme/report_heartbeat`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ factory: "sandcastle", host: "build-1", state, note, interval_seconds: 30, chat: true }),
      });
    const status = async () => (await (await fetch(`${svc.url}/api/projects/acme`)).json()).factory_status;

    expect((await status()).status).toBe("offline");

    expect((await heartbeat("online", "2 of 3 turns running")).status).toBe(200);
    let s = await status();
    expect(s.status).toBe("online");
    expect(s.factories).toMatchObject([
      { name: "sandcastle", host: "build-1", status: "online", note: "2 of 3 turns running", interval_seconds: 30, chat: true },
    ]);

    expect((await heartbeat("stopping", "stopped on SIGTERM")).status).toBe(200);
    s = await status();
    expect(s.status).toBe("offline");
    expect(s.factories).toMatchObject([{ status: "offline", note: "stopped on SIGTERM" }]);
  });

  it("the documented plan_limits show on the Project's factory_status", async () => {
    await svc.agent("/api/projects/create_project", { key: "plan", display_name: "Plan" });
    const plan_limits = {
      observed_at: "2026-11-16T09:00:00Z",
      five_hour: { used_percent: 42, resets_at: "2026-11-16T12:00:00Z" },
      weekly: { used_percent: 7, resets_at: "2026-11-20T09:00:00Z" },
    };
    const res = await fetch(`${svc.url}/api/p/plan/report_heartbeat`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ factory: "sandcastle", host: "build-1", state: "online", interval_seconds: 30, plan_limits }),
    });
    expect(res.status).toBe(200);
    const s = (await (await fetch(`${svc.url}/api/projects/plan`)).json()).factory_status;
    expect(s.plan_limits).toEqual(plan_limits);
  });
});
