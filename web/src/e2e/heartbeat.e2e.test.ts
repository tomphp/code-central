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
        body: JSON.stringify({ factory: "sandcastle", host: "build-1", state, note, interval_seconds: 30 }),
      });
    const status = async () => (await (await fetch(`${svc.url}/api/projects/acme`)).json()).factory_status;

    expect((await status()).status).toBe("offline");

    expect((await heartbeat("online", "2 of 3 turns running")).status).toBe(200);
    let s = await status();
    expect(s.status).toBe("online");
    expect(s.factories).toMatchObject([
      { name: "sandcastle", host: "build-1", status: "online", note: "2 of 3 turns running", interval_seconds: 30 },
    ]);

    expect((await heartbeat("stopping", "stopped on SIGTERM")).status).toBe(200);
    s = await status();
    expect(s.status).toBe("offline");
    expect(s.factories).toMatchObject([{ status: "offline", note: "stopped on SIGTERM" }]);
  });
});
