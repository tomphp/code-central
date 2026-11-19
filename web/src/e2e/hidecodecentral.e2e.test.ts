// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human turns on Hide Code Central in the
// Azure Boards section of Project Options and saves it; the real service
// keeps it, and it still reads back after a reload.
// Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human saves Hide Code Central", () => {
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

  it("the toggle starts off, saves, and is still on after a reload", async () => {
    const page = await browser.newPage();
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });

    await page.goto(svc.url + "/?view=project-options&project=acme");
    const boards = page.getByRole("group", { name: "Azure Boards" });
    const toggle = boards.getByRole("checkbox", { name: "Hide Code Central" });
    await toggle.waitFor();
    expect(await toggle.isChecked()).toBe(false);

    await toggle.check();
    await boards.getByRole("button", { name: "Save" }).click();
    await boards.getByRole("status").filter({ hasText: "Saved" }).waitFor();

    await page.reload();
    await boards.getByRole("checkbox", { name: "Hide Code Central" }).waitFor();
    expect(await boards.getByRole("checkbox", { name: "Hide Code Central" }).isChecked()).toBe(true);
  });
});
