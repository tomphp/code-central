// CI view tests (code-central-258): the real App against FakeApi, at
// ?view=ci. They check what the human sees and does: opening it from the
// Inbox's Admin section on the sidebar's Project or All Projects and going
// Back, the scope picker, the CI Watchers' statuses on top, the runs'
// order and fields, ignored runs, and a run that changes pulsing.
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { CIRunView } from "../api/types";

const MINUTE = 60_000;

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Pick a colour", state: "Accepted" }); // acme-1
  return fake;
}

// tracked turns CI Tracking on for key, watching repository, with
// nightly.yml ignored.
async function tracked(fake: FakeApi, key: string, repository = `${key}/web`) {
  await fake.setCITracking(key, {
    tracking: true,
    provider: "github_actions",
    repository,
    ignored_workflows: ["nightly.yml"],
    unblock_on_push: false,
  });
}

// azure turns CI Tracking on for key, watching an Azure DevOps project.
async function azure(fake: FakeApi, key: string) {
  await fake.setCITracking(key, { tracking: true, provider: "azure_pipelines", repository: "", project_url: `https://dev.azure.com/${key}/web`, ignored_workflows: [], unblock_on_push: false });
}

// at is the fake's now moved by minutes.
const at = (fake: FakeApi, minutes: number) =>
  new Date(fake.now.getTime() + minutes * MINUTE).toISOString();

// run is a run of workflow tracked against acme-1, started minutes ago and
// still going, or finished at finished minutes from now with conclusion.
function run(
  fake: FakeApi,
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
    ticket: "acme-1",
  };
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const ci = () => screen.getByRole("main", { name: "CI" });
const nav = () => screen.getByRole("navigation", { name: "Filters" });
const scope = () => within(ci()).getByRole("combobox", { name: "Projects" });
const watchers = () =>
  within(ci()).getByRole("region", { name: "CI Watchers" });
const table = () => within(ci()).findByRole("table", { name: "CI Runs" });
// rows are the runs' rows, by name.
const rows = async () =>
  within(await table())
    .getAllByRole("row")
    .slice(1);
// cells is the text of a row's header and cells.
const cells = (row: HTMLElement) =>
  [within(row).getByRole("rowheader"), ...within(row).getAllByRole("cell")].map(
    (c) => c.textContent,
  );

describe("opening and leaving the CI view", () => {
  it("opens from the Admin section, after Usage, on the sidebar's Project, and returns to the Inbox", async () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Accepted");
    const link = within(nav()).getByRole("link", { name: "CI" });
    const usage = within(nav()).getByRole("link", { name: "Usage" });
    expect(
      usage.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING,
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
    renderApp(world(), "/?view=inbox");
    expect(within(nav()).getByRole("link", { name: "CI" })).toHaveAttribute(
      "href",
      "?view=ci",
    );
    await userEvent.click(within(nav()).getByRole("link", { name: "CI" }));
    expect(scope()).toHaveValue("");
  });

  it("changes scope with the Projects picker, keeping the address", async () => {
    renderApp(world(), "/?view=ci");
    await within(scope()).findByRole("option", { name: "Beta" });
    await userEvent.selectOptions(scope(), "beta");
    expect(window.location.search).toBe("?view=ci&project=beta");
  });
});

describe("the CI Watchers' statuses", () => {
  it("heads All Projects with each tracking Project's CI Watcher", async () => {
    const fake = world();
    await tracked(fake, "acme");
    await tracked(fake, "beta");
    await fake.reportCIWatcherStatus("acme", { state: "watching" });
    await fake.reportCIWatcherStatus("beta", {
      state: "error",
      problem: "logged_out",
      message: "gh isn't logged in.",
      fix: "Run gh auth login.",
    });
    renderApp(fake, "/?view=ci");
    expect(
      await within(ci()).findByText("Acme: Watching acme/web."),
    ).toBeInTheDocument();
    expect(watchers()).toHaveTextContent(
      "Beta: Error. gh isn't logged in.Run gh auth login.",
    );
  });

  it("shows the project URL an Azure Pipelines Project watches", async () => {
    const fake = world();
    await fake.setCITracking("acme", {
      tracking: true,
      provider: "azure_pipelines",
      repository: "",
      project_url: "https://dev.azure.com/acme/web",
      ignored_workflows: [],
      unblock_on_push: false,
    });
    await fake.reportCIWatcherStatus("acme", { state: "watching" });
    renderApp(fake, "/?view=ci");
    expect(
      await within(ci()).findByText(
        "Acme: Watching https://dev.azure.com/acme/web.",
      ),
    ).toBeInTheDocument();
  });

  it("watches the project URL of an Azure Pipelines Project", async () => {
    const fake = world();
    await azure(fake, "acme");
    await fake.reportCIWatcherStatus("acme", { state: "watching" });
    renderApp(fake, "/?view=ci&project=acme");
    expect(await within(ci()).findByText("Acme: Watching https://dev.azure.com/acme/web.")).toBeInTheDocument();
  });

  it("says when the CI Watcher hasn't reported yet", async () => {
    const fake = world();
    await tracked(fake, "acme");
    renderApp(fake, "/?view=ci&project=acme");
    expect(
      await within(ci()).findByText(
        "Acme: Starting; the CI Watcher hasn't reported yet.",
      ),
    ).toBeInTheDocument();
  });

  it("says a Project has CI Tracking off, linking its Project Options", async () => {
    renderApp(world(), "/?view=ci&project=beta");
    expect(
      await within(ci()).findByText(/CI Tracking is off in Beta/),
    ).toBeInTheDocument();
    await userEvent.click(
      within(watchers()).getByRole("link", { name: "Project Options" }),
    );
    expect(window.location.search).toBe("?view=project-options&project=beta");
  });

  it("says when no Project has CI Tracking on", async () => {
    renderApp(world(), "/?view=ci");
    expect(
      await within(ci()).findByText(/No Project has CI Tracking on/),
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
    const fake = world();
    await tracked(fake, "acme");
    await fake.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -30, { finished: -20, conclusion: "success" }),
      run(fake, 2, "lint.yml", -12, { finished: -10, conclusion: "failure" }),
      run(fake, 3, "deploy.yml", -3),
      run(fake, 4, "old.yml", -8 * 24 * 60, {
        finished: -8 * 24 * 60 + 5,
        conclusion: "success",
      }),
    ]);
    renderApp(fake, "/?view=ci&project=acme");

    const shown = await rows();
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
      "acme-1",
      "View on GitHub",
    ]);
    expect(cells(shown[1])).toEqual([
      "LINT lint.yml",
      "main",
      "2abcdef",
      "failed",
      "2m 0s",
      "acme-1",
      "View on GitHub",
    ]);
    expect(
      within(shown[2]).getByRole("link", { name: "View on GitHub" }),
    ).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");

    await userEvent.click(
      within(shown[2]).getByRole("link", { name: "acme-1" }),
    );
    expect(window.location.search).toBe(
      "?view=inbox&project=acme&ticket=acme-1",
    );
  });

  it("labels an Azure Pipelines run's link from its Project's CI Provider", async () => {
    const fake = world();
    await azure(fake, "acme");
    await tracked(fake, "beta");
    await fake.reportCIRuns("acme", [{ ...run(fake, 1, "Build", -3), url: "https://dev.azure.com/acme/web/_build/results?buildId=1" }]);
    fake.addTicket("beta", { title: "Other", state: "Accepted" });
    await fake.reportCIRuns("beta", [{ ...run(fake, 2, "ci.yml", -3), ticket: "beta-1" }]);
    renderApp(fake, "/?view=ci");
    const [acme, beta] = (await rows()).sort((a, b) => a.getAttribute("aria-label")!.localeCompare(b.getAttribute("aria-label")!));
    expect(within(acme).getByRole("link", { name: "View on Azure DevOps" })).toHaveAttribute("href", "https://dev.azure.com/acme/web/_build/results?buildId=1");
    expect(within(beta).getByRole("link", { name: "View on GitHub" })).toBeInTheDocument();
  });

  it("shows a partially succeeded run as passed with a warning mark", async () => {
    const fake = world();
    await azure(fake, "acme");
    await fake.reportCIRuns("acme", [run(fake, 1, "Build", -5, { finished: -3, conclusion: "partially_succeeded" })]);
    renderApp(fake, "/?view=ci&project=acme");
    const [row] = await rows();
    const status = within(row).getByText(/Partially succeeded/);
    expect(status).toHaveClass("ci-partially-succeeded");
    expect(status).toHaveTextContent("⚠ Partially succeeded");
    expect(within(row).getByRole("link", { name: "View on Azure DevOps" })).toBeInTheDocument();
  });

  it("names each run's Project in All Projects, and a CI Rule a run is tracked against", async () => {
    const fake = world();
    await tracked(fake, "acme");
    await fake.reportCIRuns("acme", [
      { ...run(fake, 1, "ci.yml", -3), ticket: undefined, rule: "acme-9" },
    ]);
    renderApp(fake, "/?view=ci");
    const [row] = await rows();
    expect(cells(row)).toEqual([
      "CI ci.yml",
      "main",
      "1abcdef",
      "running",
      "3m 0s",
      "CI Rule acme-9",
      "Acme",
      "View on GitHub",
    ]);
    expect(within(row).getByRole("link", { name: "acme-9" })).toHaveAttribute(
      "href",
      "?view=inbox&project=acme&requirement=acme-9",
    );
  });

  it("greys an ignored run and marks it ignored", async () => {
    const fake = world();
    await tracked(fake, "acme");
    await fake.reportCIRuns("acme", [
      run(fake, 1, "nightly.yml", -3),
      run(fake, 2, "ci.yml", -4),
    ]);
    renderApp(fake, "/?view=ci&project=acme");
    const [nightly, ciRun] = await rows();
    expect(nightly).toHaveClass("ignored");
    expect(within(nightly).getByText("ignored")).toBeInTheDocument();
    expect(ciRun).not.toHaveClass("ignored");
    expect(within(ciRun).queryByText("ignored")).toBeNull();
  });

  it("shows a cancelled run in grey and a failed or timed out run in red", async () => {
    const fake = world();
    await tracked(fake, "acme");
    await fake.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -30, { finished: -20, conclusion: "cancelled" }),
      run(fake, 2, "lint.yml", -12, { finished: -10, conclusion: "failure" }),
      run(fake, 3, "deploy.yml", -5, { finished: -4, conclusion: "timed_out" }),
    ]);
    renderApp(fake, "/?view=ci&project=acme");
    const shown = await rows();
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
    const fake = world();
    await tracked(fake, "acme");
    await fake.reportCIRuns("acme", [
      run(fake, 1, "ci.yml", -3),
      run(fake, 2, "lint.yml", -4),
    ]);
    renderApp(fake, "/?view=ci&project=acme");
    const opened = await rows();
    expect(opened.filter((r) => r.hasAttribute("data-pulsing"))).toEqual([]);

    await act(() =>
      fake.reportCIRuns("acme", [
        run(fake, 1, "ci.yml", -3, { finished: 0, conclusion: "success" }),
        run(fake, 3, "e2e.yml", -1),
      ]),
    );
    expect(
      await within(await table()).findByRole("row", { name: "E2E" }),
    ).toHaveAttribute("data-pulsing", "true");
    expect(
      within(await table()).getByRole("row", { name: "CI" }),
    ).toHaveAttribute("data-pulsing", "true");
    expect(
      within(await table()).getByRole("row", { name: "LINT" }),
    ).not.toHaveAttribute("data-pulsing");
  });
});
