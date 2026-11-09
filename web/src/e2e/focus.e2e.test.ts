// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary, for what FakeApi can't show: the real Needs
// Me and the live feed driving Focus Mode. Focus Mode is open at All clear,
// an agent posts a Round, its first Question appears with no reload, the
// human answers each Question there in order, the agent's background
// `code-central wait` exits 0, and Focus Mode moves on to the Answered
// Focus Item, then to All clear once the agent picks the answers up, and
// to the Activity Dashboard once the human chooses it over the API.
// Browser set-up as in round.e2e.test.ts.
import { spawn } from "node:child_process";
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { binary, serve, type Service } from "./serve";

describe("end-to-end: the human answers a live Round in Focus Mode", () => {
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

  it("an agent posts a Round, Focus Mode shows it live, the human answers each Question and the waiting agent wakes", async () => {
    const page = await browser.newPage();

    // Some work for the agent.
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const id: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;
    const n = id.split("-").pop();

    // The human has Focus Mode open before anything needs them.
    await page.goto(svc.url + "/?view=focus");
    const focus = page.getByRole("main", { name: "Focus Mode" });
    await focus.getByRole("region", { name: "All clear" }).waitFor();

    // The agent claims, asks, and waits in the background.
    const { claim_id } = await svc.agent("/api/p/acme/claim", { ticket: id, session_id: "e2e-session" });
    await svc.agent(`/api/p/acme/tickets/${n}/post_round`, {
      claim_id,
      questions: [
        { title: "Which database?", body: "Postgres or **SQLite**?", recommended: "SQLite" },
        { title: "Remember me?", body: "Add a remember-me box?" },
        { title: "Password rules?", body: "How long at least?", recommended: "12 characters" },
      ],
    });
    const wait = spawn(binary(), ["wait", "--project", "acme", id], {
      env: { ...process.env, CODE_CENTRAL_URL: svc.url, CODE_CENTRAL_PROJECT: "" },
      stdio: ["ignore", "pipe", "inherit"],
    });
    let waitOut = "";
    wait.stdout!.on("data", (b) => (waitOut += b));
    const waited = new Promise<number | null>((r) => wait.on("exit", r));

    // The Round's first Question appears over SSE, without a reload.
    const item = focus.getByRole("article", { name: "Focus Item" });
    const question = item.getByRole("region", { name: "Question" });
    await question.getByRole("heading", { name: "Which database?" }).waitFor();
    expect(await item.getByText(id, { exact: true }).count()).toBe(1);
    expect(await item.getByText("Login form").count()).toBe(1);
    await question.getByText("Question 1 of 3 · Round 1").waitFor();
    await focus.getByText("3 left").waitFor();
    expect(await page.title()).toBe("(1) Code Central");

    // Question 1: accept the recommendation as it stands.
    const box = question.getByRole("textbox");
    expect(await box.inputValue()).toBe("SQLite");
    await question.getByText("Recommended answer, a draft").waitFor();
    await question.getByRole("button", { name: "Answer" }).click();

    // Question 2: no recommendation, so the human types one.
    await question.getByRole("heading", { name: "Remember me?" }).waitFor();
    await question.getByText("Question 2 of 3 · Round 1").waitFor();
    await focus.getByText("2 left").waitFor();
    await box.fill("Yes, for 30 days");
    await question.getByRole("button", { name: "Answer" }).click();

    // Question 3: replace the recommendation and send it with Cmd/Ctrl+Enter.
    await question.getByRole("heading", { name: "Password rules?" }).waitFor();
    await question.getByText("Question 3 of 3 · Round 1").waitFor();
    await focus.getByText("1 left").waitFor();
    // Two answers saved: the agent is still waiting.
    expect(wait.exitCode).toBeNull();
    await box.fill("16 characters");
    await box.press("ControlOrMeta+Enter");

    // The Round is complete: the agent wakes, and Focus Mode moves on to the
    // Ticket's Answered Focus Item, its answers waiting to be picked up.
    expect(await waited).toBe(0);
    expect(JSON.parse(waitOut)).toMatchObject({ state: "Answered" });
    await item.getByRole("region", { name: "Resume or release" }).waitFor();
    await item.getByText("Answered", { exact: true }).waitFor();
    expect(await question.count()).toBe(0);

    const t = await (await fetch(`${svc.url}/api/p/acme/tickets/${n}`, { headers: { "X-Code-Central-Actor": "human" } })).json();
    expect(t.rounds[0].questions.map((q: { status: string; answer?: string }) => [q.status, q.answer])).toEqual([
      ["answered", "SQLite"],
      ["answered", "Yes, for 30 days"],
      ["answered", "16 characters"],
    ]);

    // The agent picks the answers up: nothing needs the human any more.
    await svc.agent(`/api/p/acme/tickets/${n}/pick_up_answers`, { claim_id });
    await focus.getByRole("region", { name: "All clear" }).waitFor();
    await focus.getByText("Nothing needs you.").waitFor();
    expect(await page.title()).toBe("Code Central");

    // The human chooses the Activity Dashboard over the API: the open Focus
    // Mode switches to it live, counting the agent's Ticket In Progress.
    const res = await fetch(`${svc.url}/api/set_options`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "human" },
      body: JSON.stringify({ focus_clear: "activity_dashboard" }),
    });
    expect(res.ok).toBe(true);
    const dashboard = focus.getByRole("region", { name: "Activity Dashboard" });
    await dashboard.waitFor();
    expect(await focus.getByRole("region", { name: "All clear" }).count()).toBe(0);
    await dashboard.getByRole("group", { name: "Ready" }).getByText("0", { exact: true }).waitFor();
    await dashboard.getByRole("group", { name: "In Progress" }).getByText("1", { exact: true }).waitFor();
    const entry = dashboard.getByRole("list", { name: "In Progress" }).getByRole("listitem");
    expect(await entry.count()).toBe(1);
    await entry.getByText(id, { exact: true }).waitFor();
    await entry.getByText("Login form").waitFor();
  }, 30_000);
});
