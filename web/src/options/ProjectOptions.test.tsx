// The Project Options page: opened from an active Project's menu in the
// left pane, it sets the Project's Landing Method (code-central-174) and
// its CI settings (code-central-251), whose turn-on the CI check may
// refuse (code-central-252), CI Rules (code-central-254) and the CI
// Watcher's status (code-central-253). Against
// FakeApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const page = () => screen.getByRole("main", { name: "Project Options" });
const landing = () => within(page()).getByRole("group", { name: "Landing Method" });
const ci = () => within(page()).getByRole("group", { name: "CI" });
const rules = () => within(page()).getByRole("group", { name: "CI Rules" });
const addForm = () => within(rules()).getByRole("form", { name: "Add a CI Rule" });

describe("Project Options", () => {
  it("opens from an active Project's menu, in the URL, so a reload keeps it", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const { unmount } = renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "Menu for Acme" }));
    const menu = screen.getByRole("menu", { name: "Menu for Acme" });
    expect(within(menu).getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Rename", "Options", "Archive"]);
    await userEvent.click(within(menu).getByRole("menuitem", { name: "Options" }));

    expect(await screen.findByRole("heading", { name: "Acme: Project Options" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=project-options&project=acme");

    unmount();
    renderApp(fake, window.location.pathname + window.location.search);
    expect(await screen.findByRole("heading", { name: "Acme: Project Options" })).toBeInTheDocument();

    await userEvent.click(within(page()).getByRole("link", { name: "Back to the inbox" }));
    expect(await within(nav()).findByRole("button", { name: /^Acme/ })).toHaveAttribute("aria-pressed", "true");
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });

  it("shows the current setting, with Remote enabled only under Push", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.setLandingMethod("acme", { method: "branch", push: false, remote: "upstream" });
    renderApp(fake, "/?view=project-options&project=acme");

    const merge = await within(await screen.findByRole("group", { name: "Landing Method" })).findByRole("radio", { name: "Merge into base" });
    expect(merge).not.toBeChecked();
    expect(merge).toHaveAccessibleDescription(/merges the accepted branch into the base/);
    const branch = within(landing()).getByRole("radio", { name: "Leave as a branch" });
    expect(branch).toBeChecked();
    expect(branch).toHaveAccessibleDescription(/leaves the branch for you to merge/);

    const push = within(landing()).getByRole("checkbox", { name: "Push" });
    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    expect(push).not.toBeChecked();
    expect(remote).toHaveValue("upstream");
    expect(remote).toBeDisabled();
    expect(push).toHaveAccessibleDescription(/checks the remote exists, never force-pushes, and under Merge pushes the whole base/);

    await userEvent.click(push);
    expect(remote).toBeEnabled();
  });

  it("saves the Landing Method, and shows a field error from the service", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake, "/?view=project-options&project=acme");

    const save = await within(await screen.findByRole("group", { name: "Landing Method" })).findByRole("button", { name: "Save" });
    expect(within(landing()).getByRole("radio", { name: "Merge into base" })).toBeChecked();
    expect(within(landing()).getByRole("textbox", { name: "Remote" })).toHaveValue("origin");
    expect(save).toBeDisabled();

    await userEvent.click(within(landing()).getByRole("radio", { name: "Leave as a branch" }));
    await userEvent.click(within(landing()).getByRole("checkbox", { name: "Push" }));
    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    await userEvent.clear(remote);
    await userEvent.click(save);
    expect(await within(landing()).findByRole("alert")).toHaveTextContent("Push needs a git remote");
    expect(remote).toHaveAccessibleDescription(/Push needs a git remote/);
    expect(remote).toHaveAttribute("aria-invalid", "true");
    expect((await fake.listProjects())[0].landing).toEqual({ method: "merge", push: false, remote: "origin" });

    await userEvent.type(remote, "upstream");
    expect(within(landing()).queryByRole("alert")).toBeNull();
    await userEvent.click(save);
    expect(await within(landing()).findByRole("status")).toHaveTextContent("Saved");
    expect((await fake.listProjects())[0].landing).toEqual({ method: "branch", push: true, remote: "upstream" });
    expect(save).toBeDisabled();
  });

  it("shows what was stored after a save the service tidied into no change", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.setLandingMethod("acme", { method: "merge", push: true, remote: "upstream" });
    renderApp(fake, "/?view=project-options&project=acme");

    const remote = await within(await screen.findByRole("group", { name: "Landing Method" })).findByRole("textbox", { name: "Remote" });
    await userEvent.type(remote, " ");
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    expect(await within(landing()).findByRole("status")).toHaveTextContent("Saved");
    expect(remote).toHaveValue("upstream");
    expect(within(landing()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake, "/?view=project-options&project=acme");
    expect(await within(await screen.findByRole("group", { name: "Landing Method" })).findByRole("radio", { name: "Merge into base" })).toBeChecked();

    await fake.setLandingMethod("acme", { method: "branch", push: true, remote: "upstream" });
    await waitFor(() => expect(within(landing()).getByRole("radio", { name: "Leave as a branch" })).toBeChecked());
    expect(within(landing()).getByRole("checkbox", { name: "Push" })).toBeChecked();
    expect(within(landing()).getByRole("textbox", { name: "Remote" })).toHaveValue("upstream");
  });

  it("offers no Options for an Archived Project, which takes no writes", async () => {
    const fake = new FakeApi();
    fake.addProject("old", "Old App", { archived: true });
    renderApp(fake);
    await userEvent.click(await within(nav()).findByRole("button", { name: "Menu for Old App", hidden: true }));
    expect(within(screen.getByRole("menu", { name: "Menu for Old App", hidden: true })).queryByRole("menuitem", { name: "Options", hidden: true })).toBeNull();
  });

  it("shows an Archived Project's options read-only, by its URL", async () => {
    const fake = new FakeApi();
    fake.addProject("old", "Old App", { archived: true });
    renderApp(fake, "/?view=project-options&project=old");
    expect(await within(page()).findByText(/archived: unarchive it to change its options/)).toBeInTheDocument();
    expect(within(landing()).getByRole("radio", { name: "Merge into base" })).toBeDisabled();
    expect(within(landing()).getByRole("checkbox", { name: "Push" })).toBeDisabled();
    expect(within(landing()).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(within(ci()).getByRole("checkbox", { name: "CI Tracking" })).toBeDisabled();
    expect(within(ci()).getByRole("textbox", { name: "GitHub repository" })).toBeDisabled();
    expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("says when the URL names no such Project", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake, "/?view=project-options&project=nope");
    expect(await within(page()).findByText("There is no Project nope.")).toBeInTheDocument();
    expect(within(page()).getByRole("link", { name: "Back to the inbox" })).toBeInTheDocument();
  });

  describe("the CI section", () => {
    it("shows CI Tracking off by default, on GitHub Actions, with nothing named", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");

      const group = await screen.findByRole("group", { name: "CI" });
      const tracking = within(group).getByRole("checkbox", { name: "CI Tracking" });
      expect(tracking).not.toBeChecked();
      expect(tracking).toHaveAccessibleDescription(/watches the CI of what the Factory pushes/);
      const provider = within(group).getByRole("combobox", { name: "CI Provider" });
      expect(provider).toHaveValue("github_actions");
      expect(within(provider).getAllByRole("option").map((o) => o.textContent)).toEqual(["GitHub Actions"]);
      expect(within(group).getByRole("textbox", { name: "GitHub repository" })).toHaveValue("");
      const ignored = within(group).getByRole("textbox", { name: "Ignored workflows" });
      expect(ignored).toHaveValue("");
      expect(ignored).toHaveAccessibleDescription(/Workflow file names, such as nightly\.yml/);
      expect(within(group).getByRole("checkbox", { name: "Unblock on push" })).not.toBeChecked();
      expect(within(group).getByRole("button", { name: "Save" })).toBeDisabled();
    });

    it("saves Unblock on push with the rest of the section, saying it only has effect under Merge + Push with CI Tracking on", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");

      const unblock = await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "Unblock on push" });
      expect(unblock).toHaveAccessibleDescription(/Lets Tickets blocked by a pushed Ticket start while that Ticket's CI runs or has passed/);
      expect(unblock).toHaveAccessibleDescription(/only under Merge \+ Push with CI Tracking on/);

      // It may be on while CI Tracking is off.
      await userEvent.click(unblock);
      await userEvent.type(within(ci()).getByRole("textbox", { name: "Ignored workflows" }), "nightly.yml");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      expect(await within(ci()).findByRole("status")).toHaveTextContent("Saved");
      expect((await fake.listProjects())[0].ci).toMatchObject({ tracking: false, ignored_workflows: ["nightly.yml"], unblock_on_push: true });
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(unblock);
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeEnabled();
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await waitFor(async () => expect((await fake.listProjects())[0].ci.unblock_on_push).toBe(false));
      expect((await fake.listProjects())[0].ci.ignored_workflows).toEqual(["nightly.yml"]);
    });

    it("prefills the repository from a GitHub Push remote the Factory reported, and lets the human change it", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.reportPushRemote("acme", "git@github.com:acme/web.git");
      renderApp(fake, "/?view=project-options&project=acme");

      const repository = await within(await screen.findByRole("group", { name: "CI" })).findByRole("textbox", { name: "GitHub repository" });
      expect(repository).toHaveValue("acme/web");
      expect(repository).toHaveAccessibleDescription(/From the Push remote git@github\.com:acme\/web\.git/);
      // A prefill alone is no change to save.
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(within(ci()).getByRole("checkbox", { name: "CI Tracking" }));
      await userEvent.clear(repository);
      await userEvent.type(repository, "acme/website");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      expect(await within(ci()).findByRole("status")).toHaveTextContent("Saved");
      expect((await fake.listProjects())[0].ci).toMatchObject({ tracking: true, repository: "acme/website" });
    });

    it("prefills nothing from a Push remote that isn't GitHub's, and keeps a repository already set", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      fake.addProject("beta", "Beta");
      await fake.reportPushRemote("acme", "git@gitlab.com:acme/web.git");
      await fake.reportPushRemote("beta", "https://github.com/beta/app.git");
      await fake.setCITracking("beta", { tracking: false, provider: "github_actions", repository: "beta/fork", ignored_workflows: [], unblock_on_push: false });
      const { unmount } = renderApp(fake, "/?view=project-options&project=acme");

      const repository = await within(await screen.findByRole("group", { name: "CI" })).findByRole("textbox", { name: "GitHub repository" });
      expect(repository).toHaveValue("");
      expect(repository).not.toHaveAccessibleDescription(/Push remote/);
      unmount();

      renderApp(fake, "/?view=project-options&project=beta");
      expect(await within(await screen.findByRole("group", { name: "CI" })).findByRole("textbox", { name: "GitHub repository" })).toHaveValue("beta/fork");
    });

    it("turns CI Tracking on with a prefilled repository and Ignored workflows", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.reportPushRemote("acme", "https://github.com/acme/web.git");
      renderApp(fake, "/?view=project-options&project=acme");

      const tracking = await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "CI Tracking" });
      await userEvent.click(tracking);
      await userEvent.type(within(ci()).getByRole("textbox", { name: "Ignored workflows" }), "nightly.yml, preview.yml");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      expect(await within(ci()).findByRole("status")).toHaveTextContent("Saved");
      expect((await fake.listProjects())[0].ci).toEqual({
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: ["nightly.yml", "preview.yml"],
        unblock_on_push: false,
        push_remote_url: "https://github.com/acme/web.git",
        rules: [],
        watcher: { state: "not_reported" },
      });
      expect(within(ci()).getByRole("textbox", { name: "Ignored workflows" })).toHaveValue("nightly.yml, preview.yml");
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(tracking);
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await waitFor(async () => expect((await fake.listProjects())[0].ci.tracking).toBe(false));
      expect((await fake.listProjects())[0].ci.repository).toBe("acme/web");
    });

    it("shows the CI Watcher's status while CI Tracking is on, live", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");
      await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "CI Tracking" });
      expect(within(ci()).queryByRole("region", { name: "CI Watcher" })).toBeNull();

      await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      const watcher = await within(ci()).findByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent("CI Watcher: Starting");

      await fake.reportCIWatcherStatus("acme", { state: "error", problem: "logged_out", message: "gh isn't logged in to GitHub", fix: "Run gh auth login." });
      await waitFor(() => expect(within(ci()).getByRole("region", { name: "CI Watcher" })).toHaveTextContent("CI Watcher: Error"));
      expect(within(ci()).getByRole("region", { name: "CI Watcher" })).toHaveTextContent("gh isn't logged in to GitHub");
      expect(within(ci()).getByRole("region", { name: "CI Watcher" })).toHaveTextContent("Run gh auth login.");

      await fake.reportCIWatcherStatus("acme", { state: "watching" });
      await waitFor(() => expect(within(ci()).getByRole("region", { name: "CI Watcher" })).toHaveTextContent("CI Watcher: Watching acme/web"));
      expect(within(ci()).getByRole("region", { name: "CI Watcher" })).not.toHaveTextContent("gh auth login");
    });

    it("shows until when GitHub rate-limits the CI Watcher, which is Watching, not an error", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      await fake.reportCIWatcherStatus("acme", { state: "watching", rate_limited_until: "2026-01-05T10:15:00Z" });
      renderApp(fake, "/?view=project-options&project=acme");
      const watcher = await within(await screen.findByRole("group", { name: "CI" })).findByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent("CI Watcher: Watching acme/web");
      expect(watcher).toHaveTextContent("GitHub is rate-limiting it until");
      expect(watcher.querySelector("time")).toHaveAttribute("datetime", "2026-01-05T10:15:00Z");
      expect(watcher.querySelector(".warning")).toBeNull();

      await fake.reportCIWatcherStatus("acme", { state: "watching" });
      await waitFor(() => expect(within(ci()).getByRole("region", { name: "CI Watcher" })).not.toHaveTextContent("rate-limiting"));
    });

    it("shows a crashing CI Watcher's restarts and last line, which the service restarts by itself", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [], unblock_on_push: false });
      fake.seedCIWatcherCrash("acme", 3, "panic: boom");
      renderApp(fake, "/?view=project-options&project=acme");

      const watcher = await within(await screen.findByRole("group", { name: "CI" })).findByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent("crashed, restarted 3 times; the last line it wrote: panic: boom");
      expect(watcher).toHaveTextContent("Code Central restarts it by itself");
      expect(watcher).not.toHaveTextContent("tries again every 2 minutes");
    });

    it("shows the service's refusal on its field", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");

      await userEvent.click(await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "CI Tracking" }));
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      const repository = within(ci()).getByRole("textbox", { name: "GitHub repository" });
      expect(await within(ci()).findByRole("alert")).toHaveTextContent("CI Tracking needs the GitHub repository to watch, as owner/name");
      expect(repository).toHaveAttribute("aria-invalid", "true");
      expect(repository).toHaveAccessibleDescription(/needs the GitHub repository/);
      expect((await fake.listProjects())[0].ci.tracking).toBe(false);

      await userEvent.type(repository, "acme/web");
      expect(within(ci()).queryByRole("alert")).toBeNull();
      const ignored = within(ci()).getByRole("textbox", { name: "Ignored workflows" });
      await userEvent.type(ignored, ".github/workflows/ci.yml");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      expect(await within(ci()).findByRole("alert")).toHaveTextContent("isn't a workflow file name, such as ci.yml");
      expect(ignored).toHaveAttribute("aria-invalid", "true");
    });

    it("shows a refused turn-on's reason and its fix", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      fake.seedCICheck({ reason: "gh isn't logged in to GitHub.", fix: "Run gh auth login as the user the service runs as." });
      renderApp(fake, "/?view=project-options&project=acme");

      await userEvent.click(await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "CI Tracking" }));
      await userEvent.type(within(ci()).getByRole("textbox", { name: "GitHub repository" }), "acme/web");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      const alert = await within(ci()).findByRole("alert");
      expect(alert).toHaveTextContent("gh isn't logged in to GitHub.");
      expect(alert).toHaveTextContent("Run gh auth login as the user the service runs as.");
      expect(within(ci()).getByRole("checkbox", { name: "CI Tracking" })).toHaveAccessibleDescription(/gh isn't logged in/);
      expect((await fake.listProjects())[0].ci.tracking).toBe(false);

      fake.seedCICheck();
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      expect(await within(ci()).findByRole("status")).toHaveTextContent("Saved");
      expect(within(ci()).queryByRole("alert")).toBeNull();
      expect((await fake.listProjects())[0].ci.tracking).toBe(true);
    });

    it("shows a change made elsewhere, as another tab would", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");
      expect(await within(await screen.findByRole("group", { name: "CI" })).findByRole("checkbox", { name: "CI Tracking" })).not.toBeChecked();

      await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"], unblock_on_push: true });
      await waitFor(() => expect(within(ci()).getByRole("checkbox", { name: "CI Tracking" })).toBeChecked());
      expect(within(ci()).getByRole("textbox", { name: "GitHub repository" })).toHaveValue("acme/web");
      expect(within(ci()).getByRole("textbox", { name: "Ignored workflows" })).toHaveValue("nightly.yml");
      expect(within(ci()).getByRole("checkbox", { name: "Unblock on push" })).toBeChecked();
    });
  });

  describe("the CI Rules", () => {
    it("lists the rules, each with its Spec and the State its Tickets open in", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");
      const group = await screen.findByRole("group", { name: "CI Rules" });
      expect(ci()).toContainElement(group);
      expect(within(group).getByText("No CI Rules yet.")).toBeInTheDocument();

      await fake.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Ready" });
      await fake.addCIRule("acme", { workflow: "any", branch: "release", state: "Draft" });
      const list = await within(rules()).findByRole("list", { name: "CI Rules" });
      await waitFor(() =>
        expect(within(list).getAllByRole("listitem").map((li) => li.textContent)).toEqual([
          expect.stringMatching(/^ci\.yml on main.*opens Ready Tickets on acme-1/),
          expect.stringMatching(/^any workflow on release.*opens Draft Tickets on acme-2/),
        ]),
      );
      expect(within(rules()).queryByText("No CI Rules yet.")).toBeNull();
    });

    it("adds a rule, creating its Spec", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");
      await screen.findByRole("group", { name: "CI Rules" });

      const add = within(addForm()).getByRole("button", { name: "Add rule" });
      expect(add).toBeDisabled();
      await userEvent.type(within(addForm()).getByRole("textbox", { name: "Workflow" }), "ci.yml");
      await userEvent.type(within(addForm()).getByRole("textbox", { name: "Branch" }), "main");
      await userEvent.selectOptions(within(addForm()).getByRole("combobox", { name: "Tickets open as" }), "Ready");
      await userEvent.click(add);

      expect(await within(rules()).findByText(/opens Ready Tickets on acme-1/)).toBeInTheDocument();
      expect((await fake.listProjects())[0].ci.rules).toMatchObject([{ spec: "acme-1", workflow: "ci.yml", branch: "main", state: "Ready" }]);
      expect((await fake.getSpec("acme", "acme-1")).title).toBe("CI: ci.yml on main");
      // The form is cleared for the next rule.
      expect(within(addForm()).getByRole("textbox", { name: "Workflow" })).toHaveValue("");
      expect(within(addForm()).getByRole("textbox", { name: "Branch" })).toHaveValue("");
      expect(within(addForm()).getByRole("combobox", { name: "Tickets open as" })).toHaveValue("Draft");
    });

    it("shows a refusal on the field it names", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      renderApp(fake, "/?view=project-options&project=acme");
      await screen.findByRole("group", { name: "CI Rules" });

      await userEvent.type(within(addForm()).getByRole("textbox", { name: "Workflow" }), "ci.yml");
      const branch = within(addForm()).getByRole("textbox", { name: "Branch" });
      await userEvent.type(branch, "release/*");
      await userEvent.click(within(addForm()).getByRole("button", { name: "Add rule" }));
      expect(await within(addForm()).findByRole("alert")).toHaveTextContent("a CI Rule's branch takes no globs");
      expect(branch).toHaveAttribute("aria-invalid", "true");
      expect((await fake.listProjects())[0].ci.rules).toEqual([]);

      await userEvent.clear(branch);
      expect(within(addForm()).queryByRole("alert")).toBeNull();
    });

    it("edits a rule in place, or cancels the edit", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Draft" });
      renderApp(fake, "/?view=project-options&project=acme");

      await userEvent.click(await within(await screen.findByRole("group", { name: "CI Rules" })).findByRole("button", { name: "Edit ci.yml on main" }));
      const form = within(rules()).getByRole("form", { name: "Edit ci.yml on main" });
      const workflow = within(form).getByRole("textbox", { name: "Workflow" });
      expect(workflow).toHaveValue("ci.yml");
      await userEvent.clear(workflow);
      await userEvent.type(workflow, "any");
      await userEvent.click(within(form).getByRole("button", { name: "Cancel" }));
      expect(within(rules()).queryByRole("form", { name: "Edit ci.yml on main" })).toBeNull();
      expect((await fake.listProjects())[0].ci.rules[0].workflow).toBe("ci.yml");

      await userEvent.click(within(rules()).getByRole("button", { name: "Edit ci.yml on main" }));
      const again = within(rules()).getByRole("form", { name: "Edit ci.yml on main" });
      expect(within(again).getByRole("textbox", { name: "Workflow" })).toHaveValue("ci.yml");
      await userEvent.clear(within(again).getByRole("textbox", { name: "Branch" }));
      await userEvent.type(within(again).getByRole("textbox", { name: "Branch" }), "develop");
      await userEvent.selectOptions(within(again).getByRole("combobox", { name: "Tickets open as" }), "Ready");
      await userEvent.click(within(again).getByRole("button", { name: "Save" }));

      expect(await within(rules()).findByText(/opens Ready Tickets on acme-1/)).toHaveTextContent(/^ci\.yml on develop/);
      expect(within(rules()).queryByRole("form", { name: /^Edit/ })).toBeNull();
      expect((await fake.listProjects())[0].ci.rules).toMatchObject([{ spec: "acme-1", workflow: "ci.yml", branch: "develop", state: "Ready" }]);
    });

    it("removes a rule, keeping its Spec as the record", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Draft" });
      renderApp(fake, "/?view=project-options&project=acme");

      const remove = await within(await screen.findByRole("group", { name: "CI Rules" })).findByRole("button", { name: "Remove ci.yml on main" });
      expect(remove).toHaveAccessibleDescription(/keeps its Spec/);
      await userEvent.click(remove);
      expect(await within(rules()).findByText("No CI Rules yet.")).toBeInTheDocument();
      expect((await fake.listProjects())[0].ci.rules).toEqual([]);
      expect((await fake.getSpec("acme", "acme-1")).title).toBe("CI: ci.yml on main");
    });

    it("can't be changed while the Project is archived", async () => {
      const fake = new FakeApi();
      fake.addProject("acme", "Acme");
      await fake.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Draft" });
      fake.addProject("old", "Old", { archived: true });
      renderApp(fake, "/?view=project-options&project=old");
      await screen.findByRole("group", { name: "CI Rules" });
      expect(within(addForm()).getByRole("textbox", { name: "Workflow" })).toBeDisabled();
    });
  });
});
