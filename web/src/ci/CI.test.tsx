// CI view tests (code-central-258): the real App on a WASM Core world, at
// ?view=ci. They check what the human sees and does: opening it from the
// Inbox's Admin section on the sidebar's Project or All Projects and going
// Back, the scope picker, the CI Watchers' statuses on top, the runs'
// order and fields, ignored runs, and a run that changes pulsing.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, SERVICE_LIFE_MS, WORLD_HOST, type World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import type { CIRunView } from "../api/types";
import { renderApp as renderWorldApp, scenario, settle } from "../worldTesting";

const MINUTE = 60_000;

// Fake is a world, its scenario builder and acme's Accepted Ticket, which
// the runs are tracked against.
interface Fake {
  w: World;
  s: Scenario;
  ticket: string;
}

async function world(): Promise<Fake> {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const ticket = await s.ticket("acme", { title: "Pick a colour", state: "Accepted" });
  return { w, s, ticket };
}

// tracked turns CI Tracking on for key, watching repository, with
// nightly.yml ignored.
async function tracked({ w }: Fake, key: string, repository = `${key}/web`) {
  await w.api.setCITracking(key, {
    tracking: true,
    provider: "github_actions",
    repository,
    ignored_workflows: ["nightly.yml"],
  });
}

// azure turns CI Tracking on for key, watching an Azure DevOps project.
async function azure({ w }: Fake, key: string) {
  await w.api.setCITracking(key, { tracking: true, provider: "azure_pipelines", repository: "", project_url: `https://dev.azure.com/${key}/web`, ignored_workflows: [] });
}

// at is the world's now moved by minutes.
const at = ({ w }: Fake, minutes: number) =>
  new Date(w.now().getTime() + minutes * MINUTE).toISOString();

// run is a run of workflow tracked against acme's Ticket, started minutes
// ago and still going, or finished at finished minutes from now with
// conclusion.
function run(
  fake: Fake,
  id: number,
  workflow: string,
  started: number,
  end?: { finished: number; conclusion: CIRunView["conclusion"] },
): Omit<CIRunView, "ignored"> {
  return {
    id,
    attempt: 1,
    workflow,
    name: workflow.replace(".yml", "").toUpperCase(),
    branch: "main",
    commit: `${id}abcdef0123456789`,
    status: end ? "completed" : "in_progress",
    ...(end && {
      conclusion: end.conclusion,
      finished: at(fake, end.finished),
    }),
    started: at(fake, started),
    url: `https://github.com/acme/web/actions/runs/${id}`,
    jobs: [],
    ticket: fake.ticket,
  };
}

const renderApp = ({ w }: Fake, url = "/") => renderWorldApp(w, url);

const ci = () => screen.getByRole("main", { name: "CI" });
const nav = () => screen.getByRole("navigation", { name: "Filters" });
const scope = () => within(ci()).getByRole("combobox", { name: "Projects" });
const watchers = () =>
  within(ci()).getByRole("region", { name: "CI Watchers" });
const table = () => within(ci()).getByRole("table", { name: "CI Runs" });
// rows are the runs' rows, by name.
const rows = () =>
  within(table())
    .getAllByRole("row")
    .slice(1);
// cells is the text of a row's header and cells.
const cells = (row: HTMLElement) =>
  [within(row).getByRole("rowheader"), ...within(row).getAllByRole("cell")].map(
    (c) => c.textContent,
  );

describe("opening and leaving the CI view", () => {
  it("opens from the Admin section, before Usage, on the sidebar's Project, and returns to the Inbox", async () => {
    await renderApp(await world(),"/?view=inbox&project=acme&state=Accepted");
    const link = within(nav()).getByRole("link", { name: "CI" });
    const usage = within(nav()).getByRole("link", { name: "Usage" });
    expect(
      link.compareDocumentPosition(usage) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(link).toHaveAttribute(
      "href",
      "?view=ci&project=acme&back=view%3Dinbox%26project%3Dacme%26state%3DAccepted",
    );

    await userEvent.click(link);
    expect(ci()).toBeInTheDocument();
    expect(scope()).toHaveValue("acme");

    await userEvent.click(
      within(ci()).getByRole("button", { name: "Back to the Inbox" }),
    );
    expect(window.location.search).toBe(
      "?view=inbox&project=acme&state=Accepted",
    );
  });

  it("opens on All Projects when the sidebar has none chosen", async () => {
    await renderApp(await world(),"/?view=inbox");
    expect(within(nav()).getByRole("link", { name: "CI" })).toHaveAttribute(
      "href",
      "?view=ci",
    );
    await userEvent.click(within(nav()).getByRole("link", { name: "CI" }));
    expect(scope()).toHaveValue("");
  });

  it("changes scope with the Projects picker, keeping the address", async () => {
    await renderApp(await world(),"/?view=ci");
    within(scope()).getByRole("option", { name: "Beta" });
    await userEvent.selectOptions(scope(), "beta");
    expect(window.location.search).toBe("?view=ci&project=beta");
  });
});

describe("the CI Watchers' statuses", () => {
  it("heads All Projects with each tracking Project's CI Watcher", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await tracked(fake, "beta");
    await fake.s.reportCIWatcherStatus("acme", { state: "watching" });
    await fake.s.reportCIWatcherStatus("beta", {
      state: "error",
      problem: "logged_out",
      message: "gh isn't logged in.",
      fix: "Run gh auth login.",
    });
    await renderApp(fake,"/?view=ci");
    expect(
      within(ci()).getByText("Acme: Watching acme/web."),
    ).toBeInTheDocument();
    expect(watchers()).toHaveTextContent(
      "Beta: gh isn't logged in.Run gh auth login.",
    );
  });

  it("shows the project URL an Azure Pipelines Project watches", async () => {
    const fake = await world();
    await azure(fake, "acme");
    await fake.s.reportCIWatcherStatus("acme", { state: "watching" });
    await renderApp(fake,"/?view=ci");
    expect(
      within(ci()).getByText(
        "Acme: Watching https://dev.azure.com/acme/web.",
      ),
    ).toBeInTheDocument();
  });

  it("watches the project URL of an Azure Pipelines Project", async () => {
    const fake = await world();
    await azure(fake, "acme");
    await fake.s.reportCIWatcherStatus("acme", { state: "watching" });
    await renderApp(fake,"/?view=ci&project=acme");
    expect(within(ci()).getByText("Acme: Watching https://dev.azure.com/acme/web.")).toBeInTheDocument();
  });

  it("says when the CI Watcher hasn't reported yet", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await renderApp(fake,"/?view=ci&project=acme");
    expect(
      within(ci()).getByText(
        "Acme: Starting; the CI Watcher hasn't reported yet.",
      ),
    ).toBeInTheDocument();
  });

  it("says where the CI Watcher runs, or that none does", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    fake.w.holdWatcherLease("acme", WORLD_HOST);
    fake.w.advance(SERVICE_LIFE_MS);
    fake.w.holdWatcherLease("acme", "desktop");
    await renderApp(fake, "/?view=ci&project=acme");
    expect(
      within(ci()).getByText("Acme: Runs on desktop, whose service holds its Watchers; its status shows there."),
    ).toBeInTheDocument();

    fake.w.advance(SERVICE_LIFE_MS);
    await tracked(fake, "acme", "acme/app");
    await settle(fake.w);
    expect(within(ci()).getByText("Acme: Waiting for a service to take the Watchers.")).toBeInTheDocument();
  });

  it("says a Project has CI Tracking off, linking its Project Options", async () => {
    await renderApp(await world(),"/?view=ci&project=beta");
    expect(
      within(ci()).getByText(/CI Tracking is off in Beta/),
    ).toBeInTheDocument();
    await userEvent.click(
      within(watchers()).getByRole("link", { name: "Project Options" }),
    );
    expect(window.location.search).toBe("?view=project-options&project=beta");
  });

  it("says when no Project has CI Tracking on", async () => {
    await renderApp(await world(),"/?view=ci");
    expect(
      within(ci()).getByText(/No Project has CI Tracking on/),
    ).toBeInTheDocument();
    expect(
      within(ci()).getByText(
        "No CI Runs in progress or finished in the last 7 days.",
      ),
    ).toBeInTheDocument();
  });
});

describe("the runs", () => {
  it("lists in-progress runs first, then the last 7 days' finished runs, newest first, with each row's fields", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await fake.s.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -30, { finished: -20, conclusion: "success" }),
      run(fake, 2, "lint.yml", -12, { finished: -10, conclusion: "failure" }),
      run(fake, 3, "deploy.yml", -3),
      run(fake, 4, "old.yml", -8 * 24 * 60, {
        finished: -8 * 24 * 60 + 5,
        conclusion: "success",
      }),
    ]);
    await renderApp(fake,"/?view=ci&project=acme");

    const shown = rows();
    expect(shown.map((r) => r.getAttribute("aria-label"))).toEqual([
      "DEPLOY",
      "LINT",
      "CI",
    ]);
    expect(cells(shown[0])).toEqual([
      "DEPLOY deploy.yml",
      "main",
      "3abcdef",
      "running",
      "3m 0s",
      fake.ticket,
      "View on GitHub",
    ]);
    expect(cells(shown[1])).toEqual([
      "LINT lint.yml",
      "main",
      "2abcdef",
      "failed",
      "2m 0s",
      fake.ticket,
      "View on GitHub",
    ]);
    expect(
      within(shown[2]).getByRole("link", { name: "View on GitHub" }),
    ).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");

    await userEvent.click(
      within(shown[2]).getByRole("link", { name: fake.ticket }),
    );
    expect(window.location.search).toBe(
      `?view=inbox&project=acme&ticket=${fake.ticket}`,
    );
  });

  it("labels an Azure Pipelines run's link from its Project's CI Provider", async () => {
    const fake = await world();
    await azure(fake, "acme");
    await tracked(fake, "beta");
    await fake.s.reportCIRuns("acme", [{ ...run(fake, 1, "Build", -3), url: "https://dev.azure.com/acme/web/_build/results?buildId=1" }]);
    const other = await fake.s.ticket("beta", { title: "Other", state: "Accepted" });
    await fake.s.reportCIRuns("beta", [{ ...run(fake, 2, "ci.yml", -3), ticket: other }]);
    await renderApp(fake,"/?view=ci");
    const [acme, beta] = rows().sort((a, b) => a.getAttribute("aria-label")!.localeCompare(b.getAttribute("aria-label")!));
    expect(within(acme).getByRole("link", { name: "View on Azure DevOps" })).toHaveAttribute("href", "https://dev.azure.com/acme/web/_build/results?buildId=1");
    expect(within(beta).getByRole("link", { name: "View on GitHub" })).toBeInTheDocument();
  });

  it("shows a partially succeeded run as passed with a warning mark", async () => {
    const fake = await world();
    await azure(fake, "acme");
    await fake.s.reportCIRuns("acme", [run(fake, 1, "Build", -5, { finished: -3, conclusion: "partially_succeeded" })]);
    await renderApp(fake,"/?view=ci&project=acme");
    const [row] = rows();
    const status = within(row).getByText("partially succeeded");
    expect(status).toHaveClass("ci-partially-succeeded");
    expect(within(row).getByRole("link", { name: "View on Azure DevOps" })).toBeInTheDocument();
  });

  it("names each run's Project in All Projects, and a CI Rule a run is tracked against", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    const rule = (await fake.w.api.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Draft" })).ci.rules[0].spec;
    await fake.s.reportCIRuns("acme", [
      { ...run(fake, 1, "ci.yml", -3), ticket: undefined, rule },
    ]);
    await renderApp(fake,"/?view=ci");
    const [row] = rows();
    expect(cells(row)).toEqual([
      "CI ci.yml",
      "main",
      "1abcdef",
      "running",
      "3m 0s",
      `CI Rule ${rule}`,
      "Acme",
      "View on GitHub",
    ]);
    expect(within(row).getByRole("link", { name: rule })).toHaveAttribute(
      "href",
      `?view=inbox&project=acme&requirement=${rule}`,
    );
  });

  it("greys an ignored run and marks it ignored", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await fake.s.reportCIRuns("acme", [
      run(fake, 1, "nightly.yml", -3),
      run(fake, 2, "ci.yml", -4),
    ]);
    await renderApp(fake,"/?view=ci&project=acme");
    const [nightly, ciRun] = rows();
    expect(nightly).toHaveClass("ignored");
    expect(within(nightly).getByText("ignored")).toBeInTheDocument();
    expect(ciRun).not.toHaveClass("ignored");
    expect(within(ciRun).queryByText("ignored")).toBeNull();
  });

  it("shows a cancelled run in grey and a failed or timed out run in red", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await fake.s.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -30, { finished: -20, conclusion: "cancelled" }),
      run(fake, 2, "lint.yml", -12, { finished: -10, conclusion: "failure" }),
      run(fake, 3, "deploy.yml", -5, { finished: -4, conclusion: "timed_out" }),
    ]);
    await renderApp(fake,"/?view=ci&project=acme");
    const shown = rows();
    const status = (name: string) =>
      within(
        shown.find((r) => r.getAttribute("aria-label") === name)!,
      ).getByText(/^(cancelled|failed|timed out)$/);
    expect(status("CI")).toHaveClass("ci-cancelled");
    expect(status("CI")).toHaveTextContent("cancelled");
    expect(status("LINT")).toHaveClass("ci-failed");
    expect(status("DEPLOY")).toHaveClass("ci-timed-out");
  });

  it("pulses a run that changes and one that joins, from the live feed, but not on opening", async () => {
    const fake = await world();
    await tracked(fake, "acme");
    await fake.s.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -3),
      run(fake, 2, "lint.yml", -4),
    ]);
    await renderApp(fake,"/?view=ci&project=acme");
    const opened = rows();
    expect(opened.filter((r) => r.hasAttribute("data-pulsing"))).toEqual([]);

    await fake.s.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -3, { finished: 0, conclusion: "success" }),
      run(fake, 3, "e2e.yml", -1),
    ]);
    expect(
      within(table()).getByRole("row", { name: "E2E" }),
    ).toHaveAttribute("data-pulsing", "true");
    expect(
      within(table()).getByRole("row", { name: "CI" }),
    ).toHaveAttribute("data-pulsing", "true");
    expect(
      within(table()).getByRole("row", { name: "LINT" }),
    ).not.toHaveAttribute("data-pulsing");
  });
});
