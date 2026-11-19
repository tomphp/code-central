// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human adds a Ticket from a Spec view with
// New Ticket, then, under Branch, sends an Accepted Ticket in To merge back
// to fix with a note. Browser set-up as in round.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human adds a Ticket to a Spec and sends a To merge Ticket back to fix", () => {
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

  const read = async (path: string) => (await fetch(`${svc.url}${path}`, { headers: { "X-Code-Central-Actor": "human" } })).json();

  it("adds a Ticket from a Spec view, then sends a Branch To merge Ticket back to fix", async () => {
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    await svc.human("/api/p/acme/set_landing_method", { method: "branch" });
    const spec: string = (await svc.agent("/api/p/acme/create_spec", { title: "Theming", body: "" })).id;
    const page = await browser.newPage();

    // New Ticket on the Spec view: the first line titles it, it opens Ready.
    await page.goto(`${svc.url}/?view=inbox&requirement=${spec}`);
    const pane = page.getByRole("complementary", { name: "Ticket" });
    await pane.getByRole("region", { name: "Spec" }).getByRole("button", { name: "New Ticket" }).click();
    const modal = page.getByRole("dialog", { name: "New Ticket" });
    await modal.getByRole("textbox", { name: "Ticket" }).fill("Rename the button\nCall it Save.");
    await modal.getByRole("button", { name: "Submit ticket" }).click();
    await modal.waitFor({ state: "detached" });

    const id = new URL(page.url()).searchParams.get("ticket")!;
    expect(await read(`/api/p/acme/tickets/${id.split("-").pop()}`)).toMatchObject({
      title: "Rename the button",
      description: "Rename the button\nCall it Save.",
      state: "Ready",
      spec: { id: spec },
    });

    // An Accepted Ticket under Branch waits in To merge; send it back.
    const done: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Dark palette", state: "Ready" })).id;
    const n = done.split("-").pop();
    const { claim_id } = await svc.agent("/api/p/acme/claim", { ticket: done, session_id: "e2e-session" });
    await svc.agent(`/api/p/acme/tickets/${n}/finish`, { claim_id, summary: "Done.", branch: done, commit: "abc" });
    await svc.human(`/api/p/acme/tickets/${n}/accept`, {});

    await page.goto(`${svc.url}/?view=inbox&ticket=${done}`);
    const accepted = pane.getByRole("region", { name: "Accepted" });
    const send = accepted.getByRole("button", { name: "Send back to fix" });
    await send.waitFor();
    expect(await send.isDisabled()).toBe(true);
    await accepted.getByRole("textbox", { name: "Note for the fix (required)" }).fill("Use the darker blue.");
    await send.click();

    await page.waitForFunction(async ([p]) => (await (await fetch(p, { headers: { "X-Code-Central-Actor": "human" } })).json()).state === "Ready", [`/api/p/acme/tickets/${n}`]);
    expect(await read(`/api/p/acme/tickets/${n}`)).toMatchObject({ state: "Ready" });
  }, 30_000);
});
