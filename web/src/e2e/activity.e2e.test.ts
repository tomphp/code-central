// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human opens the Activity Dashboard
// from the Inbox's Activity link while a Focus Item is waiting, sees the
// agent's In Progress Ticket there, opens it in the Inbox, and Back
// returns them to where they were.
// Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human opens the Activity Dashboard from the Inbox", () => {
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

  it("the Activity link opens the dashboard with the agent's Ticket, whatever needs the human, and Back returns", async () => {
    const page = await browser.newPage();

    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const working: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;
    const asking: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Sign-up page", state: "Ready" })).id;

    // One Ticket is In Progress, the other waits on the human with a Round.
    await svc.agent("/api/p/acme/claim", { ticket: working, session_id: "e2e-working" });
    const { claim_id } = await svc.agent("/api/p/acme/claim", { ticket: asking, session_id: "e2e-asking" });
    await svc.agent(`/api/p/acme/tickets/${asking.split("-").pop()}/post_round`, {
      claim_id,
      questions: [{ title: "Which database?", body: "Postgres or SQLite?" }],
    });

    await page.goto(svc.url + "/?view=inbox&project=acme");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Activity" }).click();

    const activity = page.getByRole("main", { name: "Activity" });
    const dashboard = activity.getByRole("region", { name: "Activity Dashboard" });
    await dashboard.waitFor();
    expect(await activity.getByRole("combobox", { name: "Projects" }).inputValue()).toBe("acme");
    const entries = dashboard.getByRole("list", { name: "In Progress" }).getByRole("listitem");
    await entries.getByText("Login form").waitFor();
    expect(await dashboard.getByRole("region", { name: "All clear" }).count()).toBe(0);

    await entries.getByText("Login form").click();
    await page.waitForURL(/view=inbox/);
    expect(page.url()).toContain(`ticket=${working}`);

    await page.goBack();
    await activity.getByRole("button", { name: "Back to the Inbox" }).click();
    await page.waitForURL(/view=inbox&project=acme$/);
  }, 30_000);
});
