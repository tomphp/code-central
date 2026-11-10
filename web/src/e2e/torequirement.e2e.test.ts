// End-to-end (`make e2e`): Playwright drives a real browser against the UI
// embedded in the built binary, with the fake gh on the service's PATH,
// for what FakeApi can't show: the real Issue Watcher bringing an issue
// in and writing back to it (code-central-290). An issue arrives as a New
// issue in Focus Mode, the human turns it To Requirement with context,
// and the Issue Watcher posts the context to the issue and labels it
// code-central:requirement. Browser set-up as in focus.e2e.test.ts.
import { chromium, type Browser } from "playwright-core";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { serve, triageRepository, type Service } from "./serve";

// The gh api path of acme/web's first page of open issues, as the Issue
// Watcher asks for it.
const OPEN_ISSUES = "repos/acme/web/issues?direction=asc&page=1&per_page=100&sort=created&state=open";

describe("end-to-end: an issue arrives and the human turns it To Requirement", () => {
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

  // writes are the fake gh's calls that write to GitHub, each as its
  // method, path and fields.
  async function writes(): Promise<string[]> {
    return (await svc.ghCalls())
      .filter((args) => args[0] === "api" && args.includes("--method"))
      .map((args) => {
        let line = `${args[args.indexOf("--method") + 1]} ${args[1]}`;
        args.forEach((a, i) => {
          if (a === "-f") line += ` ${args[i + 1]}`;
        });
        return line;
      });
  }

  // written waits for the fake gh to have been asked to write each of
  // want, failing with what it was asked if it isn't within 10 seconds.
  async function written(...want: string[]) {
    for (let deadline = Date.now() + 10_000; ; ) {
      const got = await writes();
      if (want.every((w) => got.includes(w))) return;
      if (Date.now() > deadline) expect(got).toEqual(expect.arrayContaining(want));
      await new Promise((r) => setTimeout(r, 100));
    }
  }

  it("posts the context to the issue and labels it code-central:requirement", async () => {
    const page = await browser.newPage();
    await svc.agent("/api/projects/create_project", { key: "acme", display_name: "Acme" });
    const issue = { number: 7, title: "Login is broken", body: "It fails with a 500.", html_url: "https://github.com/acme/web/issues/7", state: "open", labels: [] };
    await svc.scriptGH([triageRepository("acme/web"), { args: ["api", OPEN_ISSUES], stdout: JSON.stringify([issue]) }]);
    const on = { on: true, repository: "acme/web", label: "", new_issues_need_me: true };
    await svc.human("/api/p/acme/set_github_issues", on);

    // The Issue Watcher's first poll brings the issue in, as a New issue.
    await page.goto(svc.url + "/?view=focus");
    const item = page.getByRole("main", { name: "Focus Mode" }).getByRole("article", { name: "Focus Item" });
    const panel = item.getByRole("region", { name: "New issue" });
    await panel.waitFor({ timeout: 10_000 });

    await panel.getByRole("button", { name: "To Requirement…" }).click();
    const form = panel.getByRole("form", { name: "To Requirement" });
    await form.getByRole("textbox", { name: /Context/ }).fill("Seen on every login since Monday.");
    await form.getByRole("button", { name: "To Requirement" }).click();
    await item.waitFor({ state: "detached" });

    // The Issue Watcher polls every 2 minutes; turning GitHub Issues off
    // and on starts it again, and it polls at once.
    await svc.human("/api/p/acme/set_github_issues", { ...on, on: false });
    await svc.human("/api/p/acme/set_github_issues", on);
    const linked: { kind: string; id: string }[] = await (
      await fetch(`${svc.url}/api/p/acme/integration/linked?integration=github`, { headers: { "X-Code-Central-Actor": "integration" } })
    ).json();
    const spec = linked.find((r) => r.kind === "spec")!.id;
    // The comment links back to the UI as the service names it.
    const ui = `http://localhost:${new URL(svc.url).port}`;
    await written(
      `POST repos/acme/web/issues/7/comments body=<!-- code-central ${spec} context -->\nSeen on every login since Monday.\n\n[${spec} in Code Central](${ui}/?view=inbox&list=requirements&requirement=${spec})`,
      "POST repos/acme/web/issues/7/labels labels[]=code-central:requirement",
    );
  }, 30_000);
});
