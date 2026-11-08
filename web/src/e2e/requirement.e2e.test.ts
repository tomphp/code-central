// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human submits a Requirement from "New
// requirement" and sees it selected, waiting for a Factory to claim
// Grilling. Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human submits a Requirement in the browser", () => {
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

  it("submits a Requirement and shows it waiting for a Factory", async () => {
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const page = await browser.newPage();
    await page.goto(svc.url + "/?view=inbox");

    await page.getByRole("navigation", { name: "Filters" }).getByRole("button", { name: "New requirement" }).click();
    const modal = page.getByRole("dialog", { name: "New requirement" });
    await modal.getByRole("textbox", { name: "Requirement" }).fill("Export a Spec as Markdown\nOne file, its Tickets included.");
    await modal.getByRole("button", { name: "Submit requirement" }).click();

    const pane = page.getByRole("complementary", { name: "Ticket" });
    await pane.getByText("Waiting for a Factory to claim Grilling. Nothing needs you yet.").waitFor();
    await pane.getByRole("heading", { name: "Export a Spec as Markdown", exact: true }).waitFor();
    await modal.waitFor({ state: "detached" });

    const id = new URL(page.url()).searchParams.get("requirement")!;
    const spec = await (await fetch(`${svc.url}/api/p/acme/specs/${id.split("-").pop()}`, { headers: { "X-Code-Central-Actor": "human" } })).json();
    expect(spec).toMatchObject({ title: "Export a Spec as Markdown", requirement: { waiting_for_factory: true } });
    expect(spec.requirement.stages.map((s: { name: string }) => s.name)).toEqual(["grilling", "spec", "tickets"]);
  }, 30_000);
});
