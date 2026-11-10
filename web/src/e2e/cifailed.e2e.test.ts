// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary, for what FakeApi can't show: the real
// Needs Me working CI failed out on read and the live feed driving Focus
// Mode (code-central-261). Focus Mode is open at the Activity Dashboard,
// an Accepted Ticket's pushed commit fails CI, its CI failed Focus Item
// appears with no reload showing the CI section and its failure options,
// and the human lands it anyway: the choice is recorded for the Factory
// and Focus Mode moves back to the dashboard.
// Browser set-up as in focus.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, type Service } from "./serve";

describe("end-to-end: the human lands a CI failed Ticket anyway from Focus Mode", () => {
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

  // human makes one POST as the human, as the UI does.
  async function human(path: string, body: unknown) {
    const res = await fetch(svc.url + path, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Code-Central-Actor": "human" },
      body: JSON.stringify(body),
    });
    const v = await res.json();
    if (!res.ok) throw new Error(`${path}: ${JSON.stringify(v)}`);
    return v;
  }

  it("a pushed Ticket's CI fails, Focus Mode shows it live, and the human lands it anyway", async () => {
    const page = await browser.newPage();

    // Accepted work, under Merge + Push with CI Tracking on.
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const spec = (await svc.agent("/api/p/acme/create_spec", { title: "Login", body: "" })).id;
    const id: string = (await svc.agent("/api/p/acme/create_ticket", { spec, title: "Login form", state: "Ready" })).id;
    const n = id.split("-").pop();
    const { claim_id } = await svc.agent("/api/p/acme/claim", { ticket: id, session_id: "e2e-session" });
    await svc.agent(`/api/p/acme/tickets/${n}/finish`, { claim_id, summary: "Done.", branch: id, commit: "abc" });
    await human(`/api/p/acme/tickets/${n}/accept`, {});
    await human("/api/p/acme/set_landing_method", { method: "merge", push: true, remote: "origin" });
    await human("/api/p/acme/set_ci_tracking", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });

    // The human has Focus Mode open before anything needs them.
    await page.goto(svc.url + "/?view=focus");
    const focus = page.getByRole("main", { name: "Focus Mode" });
    await focus.getByRole("region", { name: "Activity Dashboard" }).waitFor();

    // The Factory pushes the merge, and the CI Watcher reports its run
    // failed.
    await svc.agent(`/api/p/acme/tickets/${n}/report_pushed_commit`, { commit: "abc1234567" });
    await svc.agent("/api/p/acme/report_ci_runs", {
      runs: [
        {
          id: 1,
          attempt: 1,
          workflow: "ci.yml",
          name: "CI",
          branch: "main",
          commit: "abc1234567",
          status: "completed",
          conclusion: "failure",
          url: "https://github.com/acme/web/actions/runs/1",
          jobs: [{ name: "test", conclusion: "failure", url: "https://github.com/acme/web/actions/runs/1/job/1" }],
          ticket: id,
        },
      ],
    });

    // Its CI failed Focus Item appears over SSE, without a reload.
    const item = focus.getByRole("article", { name: "Focus Item" });
    const panel = item.getByRole("region", { name: "CI failed" });
    await panel.waitFor();
    await item.getByText("CI failed", { exact: true }).first().waitFor();
    expect(await item.getByText(id, { exact: true }).count()).toBe(1);
    const ci = panel.getByRole("region", { name: "CI" });
    expect(await ci.getByRole("link", { name: "CI" }).getAttribute("href")).toBe("https://github.com/acme/web/actions/runs/1");
    await ci.getByRole("link", { name: "test" }).waitFor();
    expect(await page.title()).toBe("(1) Code Central");

    // The human lands it anyway: Needs Me is empty again, and the Ticket
    // the Factory reads carries the choice.
    await panel.getByRole("group", { name: "Failure options" }).getByRole("button", { name: "Land anyway" }).click();
    await focus.getByRole("region", { name: "Activity Dashboard" }).waitFor();
    expect(await page.title()).toBe("Code Central");
    const t = await (await fetch(`${svc.url}/api/p/acme/tickets/${n}`, { headers: { "X-Code-Central-Actor": "agent" } })).json();
    expect(t).toMatchObject({ state: "Accepted", ci_result: "failed", land_anyway: true });
    expect(t).not.toHaveProperty("needs_me_reason");
  }, 30_000);
});
