// The GitHub Issues section of Project Options (code-central-284): GitHub
// Issues off by default, its repository prefilled from the Push remote,
// the filter label and New issues need me, and a refused turn-on's
// problem and fix in place; and while it is on, the Issue Watcher's status
// (code-central-285). On the WASM Core.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";

function renderApp(w: World, url = "/?view=project-options&project=acme") {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return { w, s };
};

const page = () => screen.getByRole("main", { name: "Project Options" });
const issues = () => within(page()).getByRole("group", { name: "GitHub Issues" });
const findIssues = async () => within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "GitHub Issues" });
const field = (name: string) => within(issues()).getByRole("textbox", { name });
const toggle = (name: string) => within(issues()).getByRole("checkbox", { name });
const save = () => userEvent.click(within(issues()).getByRole("button", { name: "Save" }));
// settings is acme's GitHub Issues settings, without the Issue Watcher's
// status.
const settings = async (w: World) => {
  const { watcher: _, ...settings } = (await w.api.listProjects())[0].github_issues;
  return settings;
};
const watcher = () => within(issues()).getByRole("region", { name: "Issue Watcher" });
const findWatcher = async () => within(await findIssues()).findByRole("region", { name: "Issue Watcher" });
const turnOn = (w: World) => w.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: false });

describe("the GitHub Issues section", () => {
  it("comes after the CI section, off by default with nothing named", async () => {
    const { w } = await fresh();
    renderApp(w);

    await findIssues();
    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections).toEqual(["Landing Method", "CI", "GitHub Issues", "Models", "Stage Guides", "Auto-accept", "Focus", "Factories"]);

    const on = toggle("GitHub Issues");
    expect(on).not.toBeChecked();
    expect(on).toHaveAccessibleDescription(/Incoming Issue.*through your existing gh login/);
    expect(field("GitHub repository")).toHaveValue("");
    expect(field("Filter label")).toHaveValue("");
    expect(field("Filter label")).toHaveAccessibleDescription(/Leave it empty to bring in every open issue/);
    const needMe = toggle("New issues need me");
    expect(needMe).not.toBeChecked();
    expect(needMe).toHaveAccessibleDescription(/Needs Me/);
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("prefills the repository from a GitHub Push remote, and lets the human change it", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    renderApp(w);

    await findIssues();
    const repository = field("GitHub repository");
    expect(repository).toHaveValue("acme/web");
    expect(repository).toHaveAccessibleDescription(/From the Push remote git@github\.com:acme\/web\.git/);
    // A prefill alone is no change to save.
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.clear(repository);
    await userEvent.type(repository, "acme/tracker");
    await save();
    expect(await within(issues()).findByRole("status")).toHaveTextContent("Saved");
    expect(await settings(w)).toEqual({ on: true, repository: "acme/tracker", label: "", new_issues_need_me: false, spec: "acme-1" });
    expect(repository).not.toHaveAccessibleDescription(/Push remote/);
  });

  it("turns GitHub Issues on with a filter label and New issues need me, naming the Issues Spec", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "https://github.com/acme/web.git");
    renderApp(w);

    await findIssues();
    expect(within(issues()).queryByText(/Incoming Issues arrive on/)).toBeNull();
    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("Filter label"), " bug ");
    await userEvent.click(toggle("New issues need me"));
    await save();

    expect(await within(issues()).findByRole("status")).toHaveTextContent("Saved");
    expect(await settings(w)).toEqual({ on: true, repository: "acme/web", label: "bug", new_issues_need_me: true, spec: "acme-1" });
    expect((await w.api.getSpec("acme", "acme-1")).title).toBe("GitHub Issues: acme/web");
    expect(field("Filter label")).toHaveValue("bug");
    expect(within(issues()).getByText(/Incoming Issues arrive on acme-1/)).toBeInTheDocument();
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();

    // Turning it off keeps the settings, and the Issues Spec, for later.
    await userEvent.click(toggle("GitHub Issues"));
    await save();
    await waitFor(async () => expect((await settings(w)).on).toBe(false));
    expect(await settings(w)).toEqual({ on: false, repository: "acme/web", label: "bug", new_issues_need_me: true, spec: "acme-1" });
  });

  it("shows a refused turn-on's problem and fix in place, leaving GitHub Issues off", async () => {
    const { w } = await fresh();
    w.setIssuesCheck({
      problem: "no_triage",
      reason: "The account gh is logged in as can't triage the issues of acme/web.",
      fix: "Give the account gh is logged in as triage permission or more on the repository.",
    });
    renderApp(w);

    await findIssues();
    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("GitHub repository"), "acme/web");
    await save();
    const alert = await within(issues()).findByRole("alert");
    expect(alert).toHaveTextContent("can't triage the issues of acme/web.");
    expect(alert).toHaveTextContent("Give the account gh is logged in as triage permission");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    expect(field("GitHub repository")).toHaveAccessibleDescription(/can't triage/);
    expect(await settings(w)).toEqual({ on: false, repository: "", label: "", new_issues_need_me: false });

    w.setIssuesCheck({ problem: "logged_out", reason: "gh isn't logged in to GitHub.", fix: "Run gh auth login as the user the service runs as." });
    await save();
    await waitFor(() => expect(within(issues()).getByRole("alert")).toHaveTextContent("gh isn't logged in to GitHub."));
    expect(toggle("GitHub Issues")).toHaveAccessibleDescription(/gh isn't logged in/);
    expect(field("GitHub repository")).not.toHaveAttribute("aria-invalid");

    w.setIssuesCheck();
    await save();
    expect(await within(issues()).findByRole("status")).toHaveTextContent("Saved");
    expect(within(issues()).queryByRole("alert")).toBeNull();
    expect((await settings(w)).on).toBe(true);
  });

  it("shows the service's refusal of a missing repository on its field", async () => {
    const { w } = await fresh();
    renderApp(w);

    await findIssues();
    await userEvent.click(toggle("GitHub Issues"));
    await save();
    expect(await within(issues()).findByRole("alert")).toHaveTextContent("GitHub Issues needs the GitHub repository its issues come from, as owner/name");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    await userEvent.type(field("GitHub repository"), "acme/web");
    expect(within(issues()).queryByRole("alert")).toBeNull();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const { w } = await fresh();
    renderApp(w);
    await findIssues();

    await w.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "triage", new_issues_need_me: true });
    await waitFor(() => expect(toggle("GitHub Issues")).toBeChecked());
    expect(field("GitHub repository")).toHaveValue("acme/web");
    expect(field("Filter label")).toHaveValue("triage");
    expect(toggle("New issues need me")).toBeChecked();
  });

  it("is read-only on an Archived Project", async () => {
    const { w } = await fresh({ archived: true });
    renderApp(w);
    await findIssues();
    expect(toggle("GitHub Issues")).toBeDisabled();
    expect(field("GitHub repository")).toBeDisabled();
  });

  describe("the Issue Watcher's status", () => {
    it("shows while GitHub Issues is on, live: Starting, an error with its fix, then Watching", async () => {
      const { w, s } = await fresh();
      renderApp(w);
      await findIssues();
      expect(within(issues()).queryByRole("region", { name: "Issue Watcher" })).toBeNull();

      await turnOn(w);
      expect(await findWatcher()).toHaveTextContent("Issue Watcher: Starting; it hasn't reported yet.");

      await s.reportIssueWatcher("acme", {
        state: "error",
        problem: "no_triage",
        message: "The account gh is logged in as can't triage the issues of acme/web.",
        fix: "Give the account gh is logged in as triage permission or more on the repository.",
      });
      await waitFor(() => expect(watcher()).toHaveTextContent("Issue Watcher: Error. The account gh is logged in as can't triage the issues of acme/web."));
      expect(watcher()).toHaveTextContent("Give the account gh is logged in as triage permission or more on the repository.");
      expect(watcher()).toHaveTextContent("It tries again every 2 minutes, and goes back to Watching once the cause is fixed.");
      expect(watcher().querySelector(".warning")).not.toBeNull();

      await s.reportIssueWatcher("acme", { state: "watching" });
      await waitFor(() => expect(watcher()).toHaveTextContent("Issue Watcher: Watching acme/web."));
      expect(watcher()).not.toHaveTextContent("triage permission");

      // Turned off, it has no status to show.
      await w.api.setGitHubIssues("acme", { on: false, repository: "acme/web", label: "", new_issues_need_me: false });
      await waitFor(() => expect(within(issues()).queryByRole("region", { name: "Issue Watcher" })).toBeNull());
    });

    it("shows until when GitHub rate-limits it, as Watching, not an error", async () => {
      const { w, s } = await fresh();
      await turnOn(w);
      await s.reportIssueWatcher("acme", { state: "watching", rate_limited_until: "2026-01-05T10:15:00Z" });
      renderApp(w);

      const status = await findWatcher();
      expect(status).toHaveTextContent("Issue Watcher: Watching acme/web.");
      expect(status).toHaveTextContent("GitHub is rate-limiting it until");
      expect(status.querySelector("time")).toHaveAttribute("datetime", "2026-01-05T10:15:00Z");
      expect(status.querySelector(".warning")).toBeNull();
    });

    it("shows a crashing Issue Watcher's restarts and last line, which the service restarts by itself", async () => {
      const { w } = await fresh();
      await turnOn(w);
      w.reportWatcherCrash("issues", "acme", { restarts: 2, lastLine: "panic: boom", restartAt: new Date(w.now().getTime() + 60_000) });
      renderApp(w);

      const status = await findWatcher();
      expect(status).toHaveTextContent("Issue Watcher: Error. The Issue Watcher crashed, restarted 2 times; the last line it wrote: panic: boom");
      expect(status).toHaveTextContent("Code Central restarts it by itself");
      expect(status).not.toHaveTextContent("tries again every 2 minutes");
    });

    it("is the Issue Watcher's own, apart from the CI Watcher's", async () => {
      const { w, s } = await fresh();
      await w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
      await s.reportCIWatcherStatus("acme", { state: "error", problem: "logged_out", message: "gh isn't logged in to GitHub", fix: "Run gh auth login." });
      await turnOn(w);
      renderApp(w);

      expect(await findWatcher()).toHaveTextContent("Issue Watcher: Starting");
      expect(watcher()).not.toHaveTextContent("gh auth login");
    });
  });
});
