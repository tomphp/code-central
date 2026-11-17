// The GitHub Issues section of Project Options (code-central-284): GitHub
// Issues off by default, its repository prefilled from the Push remote,
// the filter label and New issues need me, and a refused turn-on's
// problem and fix in place; and while it is on, the Issue Watcher's status
// (code-central-285). On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return { w, s };
};

const page = () => screen.getByRole("main", { name: "Project Options" });
const issues = () => within(page()).getByRole("group", { name: "GitHub Issues" });
const field = (name: string) => within(issues()).getByRole("textbox", { name });
const toggle = (name: string) => within(issues()).getByRole("checkbox", { name });
const save = async (w: World) => {
  await userEvent.click(within(issues()).getByRole("button", { name: "Save" }));
  await settle(w);
};
// settings is acme's GitHub Issues settings, without the Issue Watcher's
// status.
const settings = async (w: World) => {
  const { github_issues } = (await w.api.listProjects())[0];
  return Object.fromEntries(Object.entries(github_issues).filter(([k]) => k !== "watcher")) as Omit<typeof github_issues, "watcher">;
};
const watcher = () => within(issues()).getByRole("region", { name: "Issue Watcher" });
const turnOn = async (w: World) => {
  await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "", new_issues_need_me: false });
  await settle(w);
};
const listing = () => within(issues()).getByRole("list", { name: "GitHub repositories" });
const row = (repository: string) => within(listing()).getByRole("listitem", { name: repository });
const add = async (repository: string) => {
  await userEvent.type(field("GitHub repository"), repository);
  await userEvent.click(within(issues()).getByRole("button", { name: "Add repository" }));
};

describe("the GitHub Issues section", () => {
  it("comes after the CI section, off by default with nothing named", async () => {
    const { w } = await fresh();
    await renderApp(w);

    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections).toEqual(["Repositories", "Landing Method", "CI", "GitHub Issues", "Models", "Stage Guides", "Auto-accept", "Focus", "Factories"]);

    const on = toggle("GitHub Issues");
    expect(on).not.toBeChecked();
    expect(on).toHaveAccessibleDescription(/Incoming Issue.*through your existing gh login/);
    expect(field("GitHub repository")).toHaveValue("");
    expect(within(listing()).queryAllByRole("listitem")).toHaveLength(0);
    expect(field("Filter label")).toHaveValue("");
    expect(field("Filter label")).toHaveAccessibleDescription(/Leave it empty to bring in every open issue/);
    const needMe = toggle("New issues need me");
    expect(needMe).not.toBeChecked();
    expect(needMe).toHaveAccessibleDescription(/Needs Me/);
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("lists the repository of a GitHub Push remote while none is set, and lets the human change it", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await renderApp(w);

    const repository = field("GitHub repository");
    expect(row("acme/web")).toBeInTheDocument();
    expect(repository).toHaveAccessibleDescription(/From the Push remote git@github\.com:acme\/web\.git/);
    // A prefill alone is no change to save.
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.click(within(row("acme/web")).getByRole("button", { name: "Remove acme/web" }));
    await add("acme/tracker");
    await save(w);
    expect(within(issues()).getByRole("status")).toHaveTextContent("Saved");
    expect(await settings(w)).toEqual({ on: true, repositories: ["acme/tracker"], label: "", new_issues_need_me: false, spec: "acme-1" });
    expect(row("acme/tracker")).toBeInTheDocument();
    expect(within(listing()).queryByRole("listitem", { name: "acme/web" })).toBeNull();
    expect(repository).not.toHaveAccessibleDescription(/Push remote/);
  });

  it("turns GitHub Issues on with a filter label and New issues need me, naming the Issues Spec", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "https://github.com/acme/web.git");
    await renderApp(w);

    expect(within(issues()).queryByText(/Incoming Issues arrive on/)).toBeNull();
    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("Filter label"), " bug ");
    await userEvent.click(toggle("New issues need me"));
    await save(w);

    expect(within(issues()).getByRole("status")).toHaveTextContent("Saved");
    expect(await settings(w)).toEqual({ on: true, repositories: ["acme/web"], label: "bug", new_issues_need_me: true, spec: "acme-1" });
    expect((await w.api.getSpec("acme", "acme-1")).title).toBe("GitHub Issues: acme/web");
    expect(field("Filter label")).toHaveValue("bug");
    expect(within(issues()).getByText(/Incoming Issues arrive on acme-1/)).toBeInTheDocument();
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();

    // Turning it off keeps the settings, and the Issues Spec, for later.
    await userEvent.click(toggle("GitHub Issues"));
    await save(w);
    expect(await settings(w)).toEqual({ on: false, repositories: ["acme/web"], label: "bug", new_issues_need_me: true, spec: "acme-1" });
  });

  it("shows a refused turn-on's problem and fix in place, leaving GitHub Issues off", async () => {
    const { w } = await fresh();
    w.setIssuesCheck({
      problem: "no_triage",
      reason: "The account gh is logged in as can't triage the issues of acme/web.",
      fix: "Give the account gh is logged in as triage permission or more on the repository.",
    });
    await renderApp(w);

    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("GitHub repository"), "acme/web");
    await save(w);
    const alert = within(issues()).getByRole("alert");
    expect(alert).toHaveTextContent("can't triage the issues of acme/web.");
    expect(alert).toHaveTextContent("Give the account gh is logged in as triage permission");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    expect(field("GitHub repository")).toHaveAccessibleDescription(/can't triage/);
    expect(await settings(w)).toEqual({ on: false, repositories: [], label: "", new_issues_need_me: false });

    w.setIssuesCheck({ problem: "logged_out", reason: "gh isn't logged in to GitHub.", fix: "Run gh auth login as the user the service runs as." });
    await save(w);
    expect(within(issues()).getByRole("alert")).toHaveTextContent("gh isn't logged in to GitHub.");
    expect(toggle("GitHub Issues")).toHaveAccessibleDescription(/gh isn't logged in/);
    expect(field("GitHub repository")).not.toHaveAttribute("aria-invalid");

    w.setIssuesCheck();
    await save(w);
    expect(within(issues()).getByRole("status")).toHaveTextContent("Saved");
    expect(within(issues()).queryByRole("alert")).toBeNull();
    expect((await settings(w)).on).toBe(true);
  });

  it("shows the service's refusal of a missing repository on its field", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await userEvent.click(toggle("GitHub Issues"));
    await save(w);
    expect(within(issues()).getByRole("alert")).toHaveTextContent("GitHub Issues needs the GitHub repositories its issues come from, as owner/name");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    await userEvent.type(field("GitHub repository"), "acme/web");
    expect(within(issues()).queryByRole("alert")).toBeNull();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "triage", new_issues_need_me: true });
    await settle(w);
    expect(toggle("GitHub Issues")).toBeChecked();
    expect(row("acme/web")).toBeInTheDocument();
    expect(field("Filter label")).toHaveValue("triage");
    expect(toggle("New issues need me")).toBeChecked();
  });

  it("is read-only on an Archived Project", async () => {
    const { w } = await fresh({ archived: true });
    await renderApp(w);
    expect(toggle("GitHub Issues")).toBeDisabled();
    expect(field("GitHub repository")).toBeDisabled();
  });

  describe("the Issue Watcher's status", () => {
    it("shows while GitHub Issues is on, live: Starting, an error with its fix, then Watching", async () => {
      const { w, s } = await fresh();
      await renderApp(w);
      expect(within(issues()).queryByRole("region", { name: "Issue Watcher" })).toBeNull();

      await turnOn(w);
      expect(watcher()).toHaveTextContent("Issue Watcher: Starting; it hasn't reported yet.");

      await s.reportIssueWatcher("acme", {
        state: "error",
        problem: "no_triage",
        message: "The account gh is logged in as can't triage the issues of acme/web.",
        fix: "Give the account gh is logged in as triage permission or more on the repository.",
      });
      expect(watcher()).toHaveTextContent("Issue Watcher: Error. The account gh is logged in as can't triage the issues of acme/web.");
      expect(watcher()).toHaveTextContent("Give the account gh is logged in as triage permission or more on the repository.");
      expect(watcher()).toHaveTextContent("It tries again every 2 minutes, and goes back to Watching once the cause is fixed.");
      expect(watcher().querySelector(".warning")).not.toBeNull();

      await s.reportIssueWatcher("acme", { state: "watching" });
      expect(watcher()).toHaveTextContent("Issue Watcher: Watching acme/web.");
      expect(watcher()).not.toHaveTextContent("triage permission");

      // Turned off, it has no status to show.
      await w.api.setGitHubIssues("acme", { on: false, repositories: ["acme/web"], label: "", new_issues_need_me: false });
      await settle(w);
      expect(within(issues()).queryByRole("region", { name: "Issue Watcher" })).toBeNull();
    });

    it("shows until when GitHub rate-limits it, as Watching, not an error", async () => {
      const { w, s } = await fresh();
      await turnOn(w);
      await s.reportIssueWatcher("acme", { state: "watching", rate_limited_until: "2026-01-05T10:15:00Z" });
      await renderApp(w);

      const status = watcher();
      expect(status).toHaveTextContent("Issue Watcher: Watching acme/web.");
      expect(status).toHaveTextContent("GitHub is rate-limiting it until");
      expect(status.querySelector("time")).toHaveAttribute("datetime", "2026-01-05T10:15:00Z");
      expect(status.querySelector(".warning")).toBeNull();
    });

    it("shows a crashing Issue Watcher's restarts and last line, which the service restarts by itself", async () => {
      const { w } = await fresh();
      await turnOn(w);
      w.reportWatcherCrash("issues", "acme", { restarts: 2, lastLine: "panic: boom", restartAt: new Date(w.now().getTime() + 60_000) });
      await renderApp(w);

      const status = watcher();
      expect(status).toHaveTextContent("Issue Watcher: Error. The Issue Watcher crashed, restarted 2 times; the last line it wrote: panic: boom");
      expect(status).toHaveTextContent("Code Central restarts it by itself");
      expect(status).not.toHaveTextContent("tries again every 2 minutes");
    });

    it("is the Issue Watcher's own, apart from the CI Watcher's", async () => {
      const { w, s } = await fresh();
      await w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
      await s.reportCIWatcherStatus("acme", { state: "error", problem: "logged_out", message: "gh isn't logged in to GitHub", fix: "Run gh auth login." });
      await turnOn(w);
      await renderApp(w);

      expect(watcher()).toHaveTextContent("Issue Watcher: Starting");
      expect(watcher()).not.toHaveTextContent("gh auth login");
    });
  });

  describe("several repositories (code-central-858)", () => {
    it("adds and removes repositories, titling the Issues Spec GitHub Issues once there are several", async () => {
      const { w } = await fresh();
      await renderApp(w);

      await userEvent.click(toggle("GitHub Issues"));
      await add("acme/web");
      await add("acme/api");
      await userEvent.type(field("GitHub repository"), "ACME/Web");
      await userEvent.click(within(issues()).getByRole("button", { name: "Add repository" }));
      // A repository listed already isn't listed twice, whatever its case.
      expect(within(listing()).getAllByRole("listitem")).toHaveLength(2);
      await save(w);
      expect(await settings(w)).toEqual({ on: true, repositories: ["acme/web", "acme/api"], label: "", new_issues_need_me: false, spec: "acme-1" });
      expect((await w.api.getSpec("acme", "acme-1")).title).toBe("GitHub Issues");

      await userEvent.click(within(row("acme/web")).getByRole("button", { name: "Remove acme/web" }));
      await save(w);
      expect((await settings(w)).repositories).toEqual(["acme/api"]);
      expect((await w.api.getSpec("acme", "acme-1")).title).toBe("GitHub Issues: acme/api");
    });

    it("offers each Repository's GitHub remote as a one-click suggestion, naming its Repository", async () => {
      const { w, s } = await fresh();
      await s.reportPushRemote("acme", "git@github.com:acme/home.git");
      await s.repository("acme", "api", "git@github.com:acme/api.git");
      await s.repository("acme", "docs", "https://dev.azure.com/org/proj/_git/docs");
      await renderApp(w);

      const suggestions = within(issues()).getByRole("list", { name: "Suggested repositories" });
      // The Home Repository's remote is listed already; Azure's isn't GitHub's.
      expect(within(suggestions).getAllByRole("listitem")).toHaveLength(1);
      expect(suggestions).toHaveTextContent("The remote of api.");
      await userEvent.click(within(suggestions).getByRole("button", { name: "Add acme/api" }));
      expect(row("acme/api")).toBeInTheDocument();
      expect(within(issues()).queryByRole("list", { name: "Suggested repositories" })).toBeNull();
    });

    it("shows a refused repository on the field, with the check's fix, leaving the list as it was", async () => {
      const { w } = await fresh();
      await turnOn(w);
      w.setIssuesCheck({
        problem: "repository_unreadable",
        reason: "acme/gone: gh can't read the repository acme/gone",
        fix: "acme/gone: Check the repository is the owner/name GitHub shows.",
        repositories: [{ repository: "acme/gone", problem: "repository_unreadable", message: "gh can't read the repository acme/gone", fix: "Check the repository is the owner/name GitHub shows." }],
      });
      await renderApp(w);

      await add("acme/gone");
      await save(w);
      const alert = within(issues()).getByRole("alert");
      expect(alert).toHaveTextContent("acme/gone: gh can't read the repository acme/gone");
      expect(alert).toHaveTextContent("Check the repository is the owner/name GitHub shows.");
      expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
      expect((await settings(w)).repositories).toEqual(["acme/web"]);
    });

    it("shows each repository's status, one failing with its own fix while the other is Watching", async () => {
      const { w, s } = await fresh();
      await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web", "acme/api"], label: "", new_issues_need_me: false });
      await renderApp(w);
      expect(row("acme/web")).toHaveTextContent("acme/web: Starting");

      await s.reportIssueWatcher("acme", {
        state: "error",
        problem: "repository_unreadable",
        message: "acme/api: gh can't read the repository acme/api",
        fix: "acme/api: Check the repository is the owner/name GitHub shows.",
        repositories: [{ repository: "acme/api", problem: "repository_unreadable", message: "gh can't read the repository acme/api", fix: "Check the repository is the owner/name GitHub shows." }],
      });
      expect(row("acme/web")).toHaveTextContent("acme/web: Watching");
      expect(row("acme/api")).toHaveTextContent("acme/api: Error. gh can't read the repository acme/api");
      expect(row("acme/api")).toHaveTextContent("Check the repository is the owner/name GitHub shows.");
      expect(row("acme/api").querySelector(".warning")).not.toBeNull();
      expect(row("acme/web").querySelector(".warning")).toBeNull();
      // The status below sums up without listing them again.
      expect(watcher()).toHaveTextContent("Issue Watcher: Error in 1 Repository");
      expect(within(watcher()).queryByRole("list")).toBeNull();

      await s.reportIssueWatcher("acme", { state: "watching" });
      expect(row("acme/api")).toHaveTextContent("acme/api: Watching");
      expect(watcher()).toHaveTextContent("Issue Watcher: Watching acme/web, acme/api.");
    });

    it("shows an error that isn't one repository's on every repository", async () => {
      const { w, s } = await fresh();
      await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web", "acme/api"], label: "", new_issues_need_me: false });
      await s.reportIssueWatcher("acme", { state: "error", problem: "logged_out", message: "gh isn't logged in to GitHub", fix: "Run gh auth login." });
      await renderApp(w);

      for (const repository of ["acme/web", "acme/api"]) {
        expect(row(repository)).toHaveTextContent("Error. gh isn't logged in to GitHub");
        expect(row(repository)).toHaveTextContent("Run gh auth login.");
      }
    });
  });
});
