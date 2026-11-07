// End-to-end test 3 (`make e2e`): Playwright drives a real browser against
// the UI embedded in the built binary. An agent posts a Round, the open
// inbox updates over SSE with no reload, and the human answers it there;
// the agent's background `code-central wait` then exits 0.
//
// It uses the installed Google Chrome (PLAYWRIGHT_CHANNEL, default
// "chrome"); set PLAYWRIGHT_CHANNEL= (empty) to use Playwright's own
// Chromium after `npx playwright-core install chromium`.
import { spawn } from "node:child_process";
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { binary, serve, type Service } from "./serve";

describe("end-to-end test 3: the human answers a Round in the browser", () => {
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

  it("an agent posts a Round, the inbox shows it live, the human answers and the waiting agent wakes", async () => {
    const page = await browser.newPage();
    const html = await (await fetch(svc.url + "/")).text();
    expect(html, "the binary must embed the built UI: build it with `make build`").toContain('id="root"');

    // Some work for the agent.
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const id: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;

    // The human has the inbox open before anything needs them.
    await page.goto(svc.url + "/?view=inbox");
    await page.getByText("Nothing needs you.").waitFor();

    // The agent claims, asks, and waits in the background.
    const { claim_id } = await svc.agent("/api/p/acme/claim", { ticket: id, session_id: "e2e-session" });
    await svc.agent(`/api/p/acme/tickets/${id.split("-").pop()}/post_round`, {
      claim_id,
      questions: [
        { title: "Which database?", body: "Postgres or SQLite?", recommended: "SQLite" },
        { title: "Remember me?", body: "Add a remember-me box?" },
      ],
    });
    const wait = spawn(binary(), ["wait", "--project", "acme", id], {
      env: { ...process.env, CODE_CENTRAL_URL: svc.url, CODE_CENTRAL_PROJECT: "" },
      stdio: ["ignore", "pipe", "inherit"],
    });
    let waitOut = "";
    wait.stdout!.on("data", (b) => (waitOut += b));
    const waited = new Promise<number | null>((r) => wait.on("exit", r));

    // The inbox updates over SSE, without a reload.
    const list = page.getByRole("main", { name: "Tickets" });
    const row = list.getByRole("link", { name: new RegExp(id) });
    await row.waitFor();
    expect(await row.textContent()).toContain("2 of 2 open · Round 1");
    expect(await page.title()).toBe("(1) Code Central");

    // The human opens it and answers in the Round panel.
    await row.click();
    const panel = page.getByRole("region", { name: "Round 1 Questions" });
    await panel.waitFor();
    const db = panel.getByRole("group", { name: /Which database\?/ });
    const remember = panel.getByRole("group", { name: /Remember me\?/ });
    expect(await db.getByRole("textbox").inputValue()).toBe("SQLite");
    await db.getByText("Recommended answer, a draft").waitFor();

    await remember.getByRole("textbox").fill("Yes, for 30 days");
    await remember.getByRole("button", { name: "Answer" }).click();
    await panel.getByText("1 of 2 open").waitFor();
    // One answer saved: the agent is still waiting.
    expect(wait.exitCode).toBeNull();

    await panel.getByRole("button", { name: "Accept all recommended" }).click();

    // The Round is complete: the agent wakes and the inbox moves the Ticket on.
    expect(await waited).toBe(0);
    expect(JSON.parse(waitOut)).toMatchObject({ state: "Answered" });
    await page.getByRole("complementary", { name: "Ticket" }).getByText("Answered", { exact: true }).first().waitFor();
    await panel.waitFor({ state: "detached" });

    const t = await (
      await fetch(`${svc.url}/api/p/acme/tickets/${id.split("-").pop()}`, { headers: { "X-Code-Central-Actor": "human" } })
    ).json();
    expect(t.rounds[0].questions.map((q: { status: string; answer?: string }) => [q.status, q.answer])).toEqual([
      ["answered", "SQLite"],
      ["answered", "Yes, for 30 days"],
    ]);
  }, 30_000);
});
