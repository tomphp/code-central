// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human opens the Board from the Inbox's
// View section, sees an agent's Ticket in its In Progress column, opens its
// Ticket popup, closes it with Escape, and after reopening it closes it with
// the browser's Back.
// Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human opens the Board from the Inbox", () => {
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

  it("the Board link opens the Board with the agent's Ticket in its column, a card opens its Ticket popup, and Escape or Back closes it", async () => {
    const page = await browser.newPage();

    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const working: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;
    await svc.agent("/api/p/acme/create_ticket", { spec, title: "Sign-up page", state: "Ready" });
    await svc.agent("/api/p/acme/claim", { ticket: working, session_id: "e2e-working" });

    await page.goto(svc.url + "/?view=inbox&project=acme");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: /^Board/ }).click();

    const board = page.getByRole("main", { name: "Board" });
    const inProgress = board.getByRole("region", { name: "In Progress" });
    await inProgress.getByRole("link", { name: /Login form/ }).waitFor();
    expect(await board.getByRole("combobox", { name: "Projects" }).inputValue()).toBe("acme");
    expect(await board.getByRole("region", { name: "Ready" }).getByRole("link", { name: /Sign-up page/ }).count()).toBe(1);
    const boardAddress = page.url();

    const popup = page.getByRole("dialog", { name: "Ticket popup" });
    await inProgress.getByText("Login form").click();
    await popup.getByRole("heading", { name: /Login form/ }).waitFor();
    expect(page.url()).toContain(`popup=${working}`);

    await page.keyboard.press("Escape");
    await popup.waitFor({ state: "detached" });
    expect(page.url()).toBe(boardAddress);

    await inProgress.getByText("Login form").click();
    await popup.getByRole("heading", { name: /Login form/ }).waitFor();
    await page.goBack();
    await popup.waitFor({ state: "detached" });
    expect(page.url()).toBe(boardAddress);
  }, 30_000);
});
