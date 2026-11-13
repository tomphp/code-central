// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary. The human opens the Board from the Inbox's
// View section, sees an agent's Ticket in its In Progress column, opens its
// Ticket popup, closes it with Escape, and after reopening it closes it with
// the browser's Back. Then an agent posts a Round: its card opens the Focus
// popup, the human answers each Question there, and once the agent picks the
// answers up the popup closes by itself, back on the Board.
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

  it("an agent posts a Round, the human answers it in the card's Focus popup, which closes back to the Board once the agent picks the answers up", async () => {
    const page = await browser.newPage();

    await svc.agent("/api/projects/create_project", { key: "beta", display_name: "Beta" });
    const spec = (await svc.agent("/api/p/beta/create_spec", { title: "Shop", body: "" })).id;
    const id: string = (await svc.agent("/api/p/beta/create_ticket", { spec, title: "Basket", state: "Ready" })).id;
    const n = id.split("-").pop();
    const { claim_id } = await svc.agent("/api/p/beta/claim", { ticket: id, session_id: "e2e-asking" });
    await svc.agent(`/api/p/beta/tickets/${n}/post_round`, {
      claim_id,
      questions: [
        { title: "Which currency?", body: "Pounds or euros?", recommended: "Pounds" },
        { title: "Save the basket?", body: "Keep it between visits?" },
      ],
    });

    await page.goto(svc.url + "/?view=board&project=beta");
    const board = page.getByRole("main", { name: "Board" });
    const card = board.getByRole("region", { name: "Needs Answer" }).getByRole("link", { name: /Basket/ });
    await card.waitFor();
    const boardAddress = page.url();

    await card.getByText("Basket").click();
    const popup = page.getByRole("dialog", { name: "Focus popup" });
    const question = popup.getByRole("region", { name: "Question" });
    await question.getByRole("heading", { name: "Which currency?" }).waitFor();
    await question.getByText("Question 1 of 2 · Round 1").waitFor();
    expect(page.url()).toContain(`popup=${id}`);
    expect(await popup.getByRole("button", { name: "Skip" }).count()).toBe(0);
    expect(await popup.getByText(/\d+ left/).count()).toBe(0);
    await question.getByRole("button", { name: "Answer" }).click();

    await question.getByRole("heading", { name: "Save the basket?" }).waitFor();
    await question.getByText("Question 2 of 2 · Round 1").waitFor();
    await question.getByRole("textbox").fill("Yes");
    await question.getByRole("button", { name: "Answer" }).click();

    // The Round is complete: the popup moves on to the Ticket's Answered
    // item, and closes by itself once the agent picks the answers up.
    await popup.getByRole("region", { name: "Resume or release" }).waitFor();
    await svc.agent(`/api/p/beta/tickets/${n}/pick_up_answers`, { claim_id });
    await popup.waitFor({ state: "detached" });
    expect(page.url()).toBe(boardAddress);
    await board.getByRole("region", { name: "In Progress" }).getByRole("link", { name: /Basket/ }).waitFor();
  }, 30_000);
});
