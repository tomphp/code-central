// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human opens Usage from the Inbox and
// its Storage tab (Spec code-central-1032), and sees the service's File
// Store, each Project's records, its memory and itself as a registered
// service, with no range to pick; the scope narrows the counts.
// Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human opens Usage's Storage tab", () => {
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

  it("shows the File Store, each Project's records, the memory and the service, narrowed by the scope", async () => {
    const page = await browser.newPage();

    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    await svc.agent("/api/projects/create_project", { key: "beta", display_name: "Beta" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" });

    await page.goto(svc.url + "/?view=inbox&project=acme");
    await page.getByRole("navigation", { name: "Filters" }).getByRole("link", { name: "Usage" }).click();
    const usage = page.getByRole("main", { name: "Usage" });
    await usage.getByRole("tab", { name: "Storage" }).click();
    await page.waitForURL(/tab=storage/);

    const store = usage.getByRole("table", { name: "Store" });
    await store.waitFor();
    expect(await store.getByRole("row", { name: /^Kind/ }).getByRole("cell").textContent()).toBe("Files");
    expect(await store.getByRole("row", { name: /^Size/ }).getByRole("cell").textContent()).toMatch(/^\d+(\.\d)? (B|KB|MB)$/);
    expect(await usage.getByRole("combobox", { name: "Range" }).count()).toBe(0);
    expect(await usage.getByRole("table", { name: "Memory" }).getByRole("row", { name: /^Go heap/ }).count()).toBe(1);
    expect(await usage.getByRole("table", { name: "Services" }).getByRole("row").count()).toBe(2);

    // Beta alone, then All Projects.
    const records = usage.getByRole("table", { name: "Projects' records" }).getByRole("row");
    const scope = usage.getByRole("combobox", { name: "Projects" });
    await scope.selectOption("beta");
    await page.waitForURL(/project=beta/);
    expect(await records.allTextContents()).toEqual(["ProjectSpecsTicketsUsage days", "Beta000"]);
    await scope.selectOption("");
    await records.nth(2).waitFor();
    expect(await records.allTextContents()).toEqual(["ProjectSpecsTicketsUsage days", "Acme110", "Beta000"]);
  }, 30_000);
});
