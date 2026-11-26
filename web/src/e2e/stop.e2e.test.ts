// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. A Factory Claim is stopped from the Ticket
// view, shows in Needs Me as Stopped, and is resumed from its panel.
// Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human stops a Factory Claim and resumes it", () => {
  let svc: Service;
  let browser: Browser;
  beforeAll(async () => {
    const channel = process.env.PLAYWRIGHT_CHANNEL ?? "chrome";
    [svc, browser] = await Promise.all([serve(), chromium.launch(channel ? { channel } : {})]);
  }, 30_000);
  afterAll(async () => {
    await browser?.close();
    await svc?.stop();
  });

  it("Stop from the Ticket view lists it in Needs Me as Stopped, and Resume takes it out again", async () => {
    const page = await browser.newPage();
    page.setDefaultTimeout(8_000);
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const id: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;
    await svc.agent("/api/p/acme/claim", { ticket: id, session_id: "e2e-session", factory: "code-central-factory", host: "laptop" });

    await page.goto(`${svc.url}/?view=inbox&ticket=${id}`);
    const header = page.locator(".ticket-head");
    await header.getByRole("button", { name: "Stop", exact: true }).click();
    await header.getByRole("button", { name: "Stop session" }).click();

    const panel = page.getByRole("region", { name: "Stopped Claim" });
    await panel.waitFor();
    const needsMe = page.getByRole("main", { name: "Tickets" });
    await needsMe.getByRole("heading", { level: 3, name: "Stopped" }).waitFor();
    expect(await header.getByRole("button", { name: "Stop", exact: true }).count()).toBe(0);

    await panel.getByRole("button", { name: "Resume", exact: true }).click();
    await panel.waitFor({ state: "detached" });

    const resumed = await (await fetch(`${svc.url}/api/p/acme/tickets/${id.split("-").pop()}`)).json();
    expect(resumed).toMatchObject({ state: "In Progress", stoppable: true });
    expect(resumed.needs_me_reason).toBeUndefined();
  });
});
