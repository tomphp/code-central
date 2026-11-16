// The Project Options page: opened from an active Project's menu in the
// left pane, it sets the Project's Landing Method (code-central-174) and
// its CI settings (code-central-251), whose turn-on the CI check may
// refuse (code-central-252), CI Rules (code-central-254) and the CI
// Watcher's status (code-central-253). On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { renderApp, scenario, settle } from "../worldTesting";

// acme is a WASM Core world with the Project acme, which has no Spec.
async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const page = () => screen.getByRole("main", { name: "Project Options" });
const landing = () =>
  within(page()).getByRole("group", { name: "Landing Method" });
const ci = () => within(page()).getByRole("group", { name: "CI" });
const rules = () => within(page()).getByRole("group", { name: "CI Rules" });
const addForm = () =>
  within(rules()).getByRole("form", { name: "Add a CI Rule" });

describe("Project Options", () => {
  it("opens from an active Project's menu, in the URL, so a reload keeps it", async () => {
    const { w } = await acme();
    const { unmount } = await renderApp(w, "/");

    await userEvent.click(
      within(nav()).getByRole("button", { name: "Menu for Acme" }),
    );
    const menu = screen.getByRole("menu", { name: "Menu for Acme" });
    expect(
      within(menu)
        .getAllByRole("menuitem")
        .map((i) => i.textContent),
    ).toEqual(["Rename", "Options", "Archive"]);
    await userEvent.click(
      within(menu).getByRole("menuitem", { name: "Options" }),
    );
    await settle(w);

    expect(
      screen.getByRole("heading", { name: "Acme: Project Options" }),
    ).toBeInTheDocument();
    expect(window.location.search).toBe("?view=project-options&project=acme");

    unmount();
    await renderApp(w, window.location.pathname + window.location.search);
    expect(
      screen.getByRole("heading", { name: "Acme: Project Options" }),
    ).toBeInTheDocument();

    await userEvent.click(
      within(page()).getByRole("link", { name: "Back to the inbox" }),
    );
    await settle(w);
    expect(
      within(nav()).getByRole("button", { name: /^Acme/ }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });

  it("offers Pull Request with no Push checkbox, the worked-out Code Host, and the PAT variable only for Azure Repos", async () => {
    const { w, s } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");

    await userEvent.click(
      within(landing()).getByRole("radio", { name: "Pull Request" }),
    );
    expect(within(landing()).queryByRole("checkbox", { name: "Push" })).toBeNull();
    expect(within(landing()).getByRole("checkbox", { name: "Land per Spec" })).toBeEnabled();
    expect(within(landing()).getByRole("status", { name: "Code Host" })).toHaveTextContent(
      "start the Factory once",
    );
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    const alert = within(landing()).getByRole("alert");
    expect(alert).toHaveTextContent("Push remote's URL");
    expect(alert).toHaveTextContent("start the Factory once");

    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(landing()).getByRole("status", { name: "Code Host" })).toHaveTextContent(
      "Code Host: GitHub, repository acme/web.",
    );
    expect(within(landing()).queryByRole("textbox", { name: "PAT variable" })).toBeNull();
    expect((await w.api.listProjects())[0].landing).toMatchObject({ method: "pull_request", push: false });

    await s.reportPushRemote("acme", "https://dev.azure.com/acme/shop/_git/web");
    const pat = within(landing()).getByRole("textbox", { name: "PAT variable" });
    expect(pat).toHaveValue("ADO_PAT");
    expect(within(landing()).getByRole("status", { name: "Code Host" })).toHaveTextContent(
      "Azure Repos, repository acme/shop/web",
    );
    await userEvent.clear(pat);
    await userEvent.type(pat, "REPOS_PAT");
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing.pat_env).toBe("REPOS_PAT");
  });

  it("saves Pull Request for an Azure Repos Push remote without sending the view-only fields", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "https://dev.azure.com/org/proj/_git/repo");
    await renderApp(w, "/?view=project-options&project=acme");

    await userEvent.click(within(landing()).getByRole("radio", { name: "Pull Request" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing).toMatchObject({
      method: "pull_request",
      code_host: "azure_repos",
    });
    expect(within(landing()).queryByRole("alert")).toBeNull();
  });

  it("shows the Pull Request Watcher's status under Pull Request: Watching, rate-limited, or an error with its fix", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await renderApp(w, "/?view=project-options&project=acme");
    expect(within(landing()).queryByRole("region", { name: "Pull Request Watcher" })).toBeNull();

    await userEvent.click(within(landing()).getByRole("radio", { name: "Pull Request" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    const watcher = within(landing()).getByRole("region", { name: "Pull Request Watcher" });
    expect(watcher).toHaveTextContent("Starting; it hasn't reported yet");

    await s.reportPullRequestWatcher("acme", { state: "watching" });
    expect(watcher).toHaveTextContent("Pull Request Watcher: Watching acme/web.");

    await s.reportPullRequestWatcher("acme", {
      state: "error",
      problem: "no_write",
      message: "the account gh is logged in as can't write to acme/web",
      fix: "Log gh in as an account with write access.",
    });
    expect(watcher).toHaveTextContent("can't write to acme/web");
    expect(watcher).toHaveTextContent("Log gh in as an account with write access.");
    expect(watcher).toHaveTextContent("goes back to Watching once the cause is fixed");

    // Switching away forgets its status and hides it.
    await userEvent.click(within(landing()).getByRole("radio", { name: "Merge" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(landing()).queryByRole("region", { name: "Pull Request Watcher" })).toBeNull();
    expect((await w.api.listProjects())[0].landing.watcher).toEqual({ state: "not_reported" });
  });

  it("shows Land per Spec on under Merge, disabled but kept under Branch, and saves it", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");

    const box = within(landing()).getByRole("checkbox", { name: "Land per Spec" });
    expect(box).toBeChecked();
    expect(box).toBeEnabled();

    await userEvent.click(box);
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing.land_per_spec).toBe(false);

    await userEvent.click(
      within(landing()).getByRole("radio", { name: "Leave as a branch" }),
    );
    expect(box).toBeDisabled();
    expect(box).not.toBeChecked();
  });

  it("shows the current setting, with Remote enabled only under Push", async () => {
    const { w } = await acme();
    await w.api.setLandingMethod("acme", {
      method: "branch",
      push: false,
      remote: "upstream",
    });
    await renderApp(w, "/?view=project-options&project=acme");

    const merge = within(landing()).getByRole("radio", { name: "Merge" });
    expect(merge).not.toBeChecked();
    expect(merge).toHaveAccessibleDescription(
      /merges the accepted branch into the base/,
    );
    const branch = within(landing()).getByRole("radio", {
      name: "Leave as a branch",
    });
    expect(branch).toBeChecked();
    expect(branch).toHaveAccessibleDescription(
      /leaves the branch for you to merge/,
    );

    const push = within(landing()).getByRole("checkbox", { name: "Push" });
    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    expect(push).not.toBeChecked();
    expect(remote).toHaveValue("upstream");
    expect(remote).toBeDisabled();
    expect(push).toHaveAccessibleDescription(
      /checks the remote exists, never force-pushes, and under Merge pushes the whole base/,
    );

    await userEvent.click(push);
    expect(remote).toBeEnabled();
  });

  it("shows the Base branch above the radios, sends it on every save, and shows its error", async () => {
    const { w } = await acme();
    await w.api.setLandingMethod("acme", { method: "merge", push: false, remote: "origin", base: "develop" });
    await renderApp(w, "/?view=project-options&project=acme");

    const base = within(landing()).getByRole("textbox", { name: "Base branch" });
    expect(base).toHaveValue("develop");
    const radio = within(landing()).getByRole("radio", { name: "Merge" });
    expect(base.compareDocumentPosition(radio) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(within(landing()).queryByRole("radio", { name: "Merge into base" })).toBeNull();

    // Changing only the method still sends the base shown.
    await userEvent.click(within(landing()).getByRole("radio", { name: "Leave as a branch" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing).toMatchObject({ method: "branch", base: "develop" });

    await userEvent.clear(base);
    await userEvent.type(base, "release/1.0");
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing.base).toBe("release/1.0");

    await userEvent.clear(base);
    await userEvent.type(base, "bad name");
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(landing()).getByRole("alert")).toHaveTextContent("branch name");
    expect(base).toHaveAttribute("aria-invalid", "true");
    expect(base).toHaveAccessibleDescription(/branch name/);
    expect((await w.api.listProjects())[0].landing.base).toBe("release/1.0");
  });

  it("saves the Landing Method, and shows a field error from the service", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");

    const save = within(landing()).getByRole("button", { name: "Save" });
    expect(
      within(landing()).getByRole("radio", { name: "Merge" }),
    ).toBeChecked();
    expect(
      within(landing()).getByRole("textbox", { name: "Remote" }),
    ).toHaveValue("origin");
    expect(save).toBeDisabled();

    await userEvent.click(
      within(landing()).getByRole("radio", { name: "Leave as a branch" }),
    );
    await userEvent.click(
      within(landing()).getByRole("checkbox", { name: "Push" }),
    );
    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    await userEvent.clear(remote);
    await userEvent.click(save);
    await settle(w);
    expect(within(landing()).getByRole("alert")).toHaveTextContent(
      "Push needs a git remote",
    );
    expect(remote).toHaveAccessibleDescription(/Push needs a git remote/);
    expect(remote).toHaveAttribute("aria-invalid", "true");
    expect((await w.api.listProjects())[0].landing).toEqual({
      method: "merge",
      push: false,
      remote: "origin",
      land_per_spec: true,
      code_host: "",
      repository: "",
      pat_env: "ADO_PAT",
      base: "main",
      watcher: { state: "not_reported" },
      open_pull_requests: [],
    });

    await userEvent.type(remote, "upstream");
    expect(within(landing()).queryByRole("alert")).toBeNull();
    await userEvent.click(save);
    await settle(w);
    expect(within(landing()).getByRole("status")).toHaveTextContent("Saved");
    expect((await w.api.listProjects())[0].landing).toEqual({
      method: "branch",
      push: true,
      remote: "upstream",
      land_per_spec: true,
      code_host: "",
      repository: "",
      pat_env: "ADO_PAT",
      base: "main",
      watcher: { state: "not_reported" },
      open_pull_requests: [],
    });
    expect(save).toBeDisabled();
  });

  it("shows what was stored after a save the service tidied into no change", async () => {
    const { w } = await acme();
    await w.api.setLandingMethod("acme", {
      method: "merge",
      push: true,
      remote: "upstream",
    });
    await renderApp(w, "/?view=project-options&project=acme");

    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    await userEvent.type(remote, " ");
    await userEvent.click(
      within(landing()).getByRole("button", { name: "Save" }),
    );
    await settle(w);
    expect(within(landing()).getByRole("status")).toHaveTextContent("Saved");
    expect(remote).toHaveValue("upstream");
    expect(
      within(landing()).getByRole("button", { name: "Save" }),
    ).toBeDisabled();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");
    expect(
      within(landing()).getByRole("radio", { name: "Merge" }),
    ).toBeChecked();

    await w.api.setLandingMethod("acme", {
      method: "branch",
      push: true,
      remote: "upstream",
    });
    await settle(w);
    expect(
      within(landing()).getByRole("radio", { name: "Leave as a branch" }),
    ).toBeChecked();
    expect(
      within(landing()).getByRole("checkbox", { name: "Push" }),
    ).toBeChecked();
    expect(
      within(landing()).getByRole("textbox", { name: "Remote" }),
    ).toHaveValue("upstream");
  });

  it("offers no Options for an Archived Project, which takes no writes", async () => {
    const { w, s } = await acme();
    await s.project("old", "Old App");
    await s.archive("old");
    await renderApp(w, "/");
    await userEvent.click(
      within(nav()).getByRole("button", {
        name: "Menu for Old App",
        hidden: true,
      }),
    );
    expect(
      within(
        screen.getByRole("menu", { name: "Menu for Old App", hidden: true }),
      ).queryByRole("menuitem", { name: "Options", hidden: true }),
    ).toBeNull();
  });

  it("shows an Archived Project's options read-only, by its URL", async () => {
    const { w, s } = await acme();
    await s.project("old", "Old App");
    await s.archive("old");
    await renderApp(w, "/?view=project-options&project=old");
    expect(
      within(page()).getByText(/archived: unarchive it to change its options/),
    ).toBeInTheDocument();
    expect(
      within(landing()).getByRole("radio", { name: "Merge" }),
    ).toBeDisabled();
    expect(
      within(landing()).getByRole("checkbox", { name: "Push" }),
    ).toBeDisabled();
    expect(
      within(landing()).getByRole("button", { name: "Save" }),
    ).toBeDisabled();
    expect(
      within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
    ).toBeDisabled();
    expect(
      within(ci()).getByRole("textbox", { name: "GitHub repository" }),
    ).toBeDisabled();
    expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("says when the URL names no such Project", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=nope");
    expect(
      within(page()).getByText("There is no Project nope."),
    ).toBeInTheDocument();
    expect(
      within(page()).getByRole("link", { name: "Back to the inbox" }),
    ).toBeInTheDocument();
  });

  describe("the CI section", () => {
    it("shows CI Tracking off by default, on GitHub Actions, with nothing named", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");

      const group = screen.getByRole("group", { name: "CI" });
      const tracking = within(group).getByRole("checkbox", {
        name: "CI Tracking",
      });
      expect(tracking).not.toBeChecked();
      expect(tracking).toHaveAccessibleDescription(
        /watches the CI of what the Factory pushes/,
      );
      const provider = within(group).getByRole("combobox", {
        name: "CI Provider",
      });
      expect(provider).toHaveValue("github_actions");
      expect(
        within(provider)
          .getAllByRole("option")
          .map((o) => o.textContent),
      ).toEqual(["GitHub Actions", "Azure Pipelines"]);
      expect(
        within(group).getByRole("textbox", { name: "GitHub repository" }),
      ).toHaveValue("");
      const ignored = within(group).getByRole("textbox", {
        name: "Ignored workflows",
      });
      expect(ignored).toHaveValue("");
      expect(ignored).toHaveAccessibleDescription(
        /Workflow file names, such as nightly\.yml/,
      );
      expect(
        within(group).queryByRole("checkbox", { name: "Unblock on push" }),
      ).not.toBeInTheDocument();
      expect(
        within(group).getByRole("button", { name: "Save" }),
      ).toBeDisabled();
    });

    it("prefills the repository from a GitHub Push remote the Factory reported, and lets the human change it", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote("acme", "git@github.com:acme/web.git");
      await renderApp(w, "/?view=project-options&project=acme");

      const repository = within(ci()).getByRole("textbox", {
        name: "GitHub repository",
      });
      expect(repository).toHaveValue("acme/web");
      expect(repository).toHaveAccessibleDescription(
        /From the Push remote git@github\.com:acme\/web\.git/,
      );
      // A prefill alone is no change to save.
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      );
      await userEvent.clear(repository);
      await userEvent.type(repository, "acme/website");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(within(ci()).getByRole("status")).toHaveTextContent("Saved");
      expect((await w.api.listProjects())[0].ci).toMatchObject({
        tracking: true,
        repository: "acme/website",
      });
    });

    it("prefills nothing from a Push remote that isn't GitHub's, and keeps a repository already set", async () => {
      const { w, s } = await acme();
      await s.project("beta", "Beta");
      await s.reportPushRemote("acme", "git@gitlab.com:acme/web.git");
      await s.reportPushRemote("beta", "https://github.com/beta/app.git");
      await w.api.setCITracking("beta", {
        tracking: false,
        provider: "github_actions",
        repository: "beta/fork",
        ignored_workflows: [],
      });
      const { unmount } = await renderApp(
        w,
        "/?view=project-options&project=acme",
      );

      const repository = within(ci()).getByRole("textbox", {
        name: "GitHub repository",
      });
      expect(repository).toHaveValue("");
      expect(repository).not.toHaveAccessibleDescription(/Push remote/);
      unmount();

      await renderApp(w, "/?view=project-options&project=beta");
      expect(
        within(ci()).getByRole("textbox", { name: "GitHub repository" }),
      ).toHaveValue("beta/fork");
    });

    it("shows each provider's fields, keeping the other's values when switching", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      const group = screen.getByRole("group", { name: "CI" });
      expect(
        within(group).queryByRole("textbox", {
          name: "Azure DevOps project URL",
        }),
      ).toBeNull();
      expect(
        within(group).queryByRole("textbox", {
          name: "PAT environment variable",
        }),
      ).toBeNull();

      await userEvent.type(
        within(group).getByRole("textbox", { name: "GitHub repository" }),
        "acme/web",
      );
      await userEvent.selectOptions(
        within(group).getByRole("combobox", { name: "CI Provider" }),
        "azure_pipelines",
      );
      expect(
        within(group).queryByRole("textbox", { name: "GitHub repository" }),
      ).toBeNull();
      const url = within(group).getByRole("textbox", {
        name: "Azure DevOps project URL",
      });
      const pat = within(group).getByRole("textbox", {
        name: "PAT environment variable",
      });
      expect(url).toHaveValue("");
      expect(url).toHaveAccessibleDescription(
        /The Azure DevOps project to watch, as its URL, such as https:\/\/dev\.azure\.com\/acme\/web; CI Tracking needs one\./,
      );
      expect(pat).toHaveValue("ADO_PAT");
      expect(pat).toHaveAccessibleDescription(
        /Build: Read & execute, in the service's environment\. Code Central keeps the name, never the PAT\./,
      );
      expect(
        within(group).getByRole("textbox", { name: "Ignored workflows" }),
      ).toHaveAccessibleDescription(/Pipeline names/);

      await userEvent.type(url, "https://dev.azure.com/acme/web/_build");
      await userEvent.clear(pat);
      await userEvent.type(pat, "ACME_PAT");
      await userEvent.click(
        within(group).getByRole("button", { name: "Save" }),
      );
      await settle(w);
      expect(within(group).getByRole("status")).toHaveTextContent("Saved");
      expect((await w.api.listProjects())[0].ci).toMatchObject({
        provider: "azure_pipelines",
        repository: "acme/web",
        project_url: "https://dev.azure.com/acme/web",
        pat_env: "ACME_PAT",
      });

      await userEvent.selectOptions(
        within(group).getByRole("combobox", { name: "CI Provider" }),
        "github_actions",
      );
      expect(
        within(group).getByRole("textbox", { name: "GitHub repository" }),
      ).toHaveValue("acme/web");
    });

    it("prefills the project URL from an Azure Repos Push remote, only for Azure Pipelines with none stored", async () => {
      const { w, s } = await acme();
      await s.project("beta", "Beta");
      await s.reportPushRemote(
        "acme",
        "git@ssh.dev.azure.com:v3/acme/web/app",
      );
      await s.reportPushRemote(
        "beta",
        "https://dev.azure.com/beta/app/_git/app",
      );
      await w.api.setCITracking("beta", {
        tracking: false,
        provider: "azure_pipelines",
        repository: "",
        project_url: "https://dev.azure.com/beta/other",
        ignored_workflows: [],
      });
      const { unmount } = await renderApp(
        w,
        "/?view=project-options&project=acme",
      );

      const group = screen.getByRole("group", { name: "CI" });
      // GitHub Actions: nothing to prefill.
      await userEvent.selectOptions(
        within(group).getByRole("combobox", { name: "CI Provider" }),
        "azure_pipelines",
      );
      const url = within(group).getByRole("textbox", {
        name: "Azure DevOps project URL",
      });
      expect(url).toHaveValue("https://dev.azure.com/acme/web");
      expect(url).toHaveAccessibleDescription(
        /From the Push remote git@ssh\.dev\.azure\.com:v3\/acme\/web\/app\. The Azure DevOps project to watch/,
      );
      unmount();

      await renderApp(w, "/?view=project-options&project=beta");
      const stored = within(ci()).getByRole("textbox", {
        name: "Azure DevOps project URL",
      });
      expect(stored).toHaveValue("https://dev.azure.com/beta/other");
      expect(stored).not.toHaveAccessibleDescription(/Push remote/);
    });

    it("prefills nothing from a Server SSH Push remote", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote(
        "acme",
        "ssh://git@tfs.example.com:22/tfs/DefaultCollection/web/_git/app",
      );
      await renderApp(w, "/?view=project-options&project=acme");
      const group = screen.getByRole("group", { name: "CI" });
      await userEvent.selectOptions(
        within(group).getByRole("combobox", { name: "CI Provider" }),
        "azure_pipelines",
      );
      const url = within(group).getByRole("textbox", {
        name: "Azure DevOps project URL",
      });
      expect(url).toHaveValue("");
      expect(url).not.toHaveAccessibleDescription(/Push remote/);
    });

    it("shows field errors on the project URL and the PAT environment variable", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      const group = screen.getByRole("group", { name: "CI" });
      await userEvent.selectOptions(
        within(group).getByRole("combobox", { name: "CI Provider" }),
        "azure_pipelines",
      );
      await userEvent.click(
        within(group).getByRole("checkbox", { name: "CI Tracking" }),
      );
      await userEvent.click(
        within(group).getByRole("button", { name: "Save" }),
      );
      await settle(w);
      const url = within(group).getByRole("textbox", {
        name: "Azure DevOps project URL",
      });
      expect(url).toBeInvalid();
      expect(within(group).getByRole("alert")).toHaveTextContent(
        "CI Tracking needs the Azure DevOps project to watch, as its URL",
      );

      await userEvent.type(url, "https://dev.azure.com/acme/web");
      const pat = within(group).getByRole("textbox", {
        name: "PAT environment variable",
      });
      await userEvent.clear(pat);
      await userEvent.type(pat, "MY-PAT");
      await userEvent.click(
        within(group).getByRole("button", { name: "Save" }),
      );
      await settle(w);
      expect(pat).toBeInvalid();
      expect(url).not.toBeInvalid();
    });

    it("shows Watching the project URL for Azure Pipelines", async () => {
      const { w, s } = await acme();
      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "azure_pipelines",
        repository: "",
        project_url: "https://dev.azure.com/acme/web",
        ignored_workflows: [],
      });
      await s.reportCIWatcherStatus("acme", { state: "watching" });
      await renderApp(w, "/?view=project-options&project=acme");
      const watcher = within(ci()).getByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent(
        "CI Watcher: Watching https://dev.azure.com/acme/web.",
      );
    });

    it("turns CI Tracking on with a prefilled repository and Ignored workflows", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote("acme", "https://github.com/acme/web.git");
      await renderApp(w, "/?view=project-options&project=acme");

      const tracking = within(ci()).getByRole("checkbox", {
        name: "CI Tracking",
      });
      await userEvent.click(tracking);
      await userEvent.type(
        within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
        "nightly.yml, preview.yml",
      );
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(within(ci()).getByRole("status")).toHaveTextContent("Saved");
      expect((await w.api.listProjects())[0].ci).toEqual({
        tracking: true,
        provider: "github_actions",
        project_url: "",
        pat_env: "ADO_PAT",
        repository: "acme/web",
        ignored_workflows: ["nightly.yml", "preview.yml"],
        push_remote_url: "https://github.com/acme/web.git",
        rules: [],
        watcher: { state: "not_reported" },
      });
      expect(
        within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
      ).toHaveValue("nightly.yml, preview.yml");
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(tracking);
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect((await w.api.listProjects())[0].ci.tracking).toBe(false);
      expect((await w.api.listProjects())[0].ci.repository).toBe("acme/web");
    });

    it("shows the CI Watcher's status while CI Tracking is on, live", async () => {
      const { w, s } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      expect(
        within(ci()).queryByRole("region", { name: "CI Watcher" }),
      ).toBeNull();

      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: [],
      });
      await settle(w);
      const watcher = within(ci()).getByRole("region", {
        name: "CI Watcher",
      });
      expect(watcher).toHaveTextContent("CI Watcher: Starting");

      await s.reportCIWatcherStatus("acme", {
        state: "error",
        problem: "logged_out",
        message: "gh isn't logged in to GitHub",
        fix: "Run gh auth login.",
      });
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).toHaveTextContent("CI Watcher: Error");
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).toHaveTextContent("gh isn't logged in to GitHub");
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).toHaveTextContent("Run gh auth login.");

      await s.reportCIWatcherStatus("acme", { state: "watching" });
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).toHaveTextContent("CI Watcher: Watching acme/web");
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).not.toHaveTextContent("gh auth login");
    });

    it("shows until when GitHub rate-limits the CI Watcher, which is Watching, not an error", async () => {
      const { w, s } = await acme();
      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: [],
      });
      await s.reportCIWatcherStatus("acme", {
        state: "watching",
        rate_limited_until: "2026-01-05T10:15:00Z",
      });
      await renderApp(w, "/?view=project-options&project=acme");
      const watcher = within(ci()).getByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent("CI Watcher: Watching acme/web");
      expect(watcher).toHaveTextContent("GitHub is rate-limiting it until");
      expect(watcher.querySelector("time")).toHaveAttribute(
        "datetime",
        "2026-01-05T10:15:00Z",
      );
      expect(watcher.querySelector(".warning")).toBeNull();

      await s.reportCIWatcherStatus("acme", { state: "watching" });
      expect(
        within(ci()).getByRole("region", { name: "CI Watcher" }),
      ).not.toHaveTextContent("rate-limiting");
    });

    it("shows a crashing CI Watcher's restarts and last line, which the service restarts by itself", async () => {
      const { w } = await acme();
      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: [],
      });
      w.reportWatcherCrash("ci", "acme", {
        restarts: 3,
        lastLine: "panic: boom",
        restartAt: new Date(w.now().getTime() + 2 * 60_000),
      });
      await renderApp(w, "/?view=project-options&project=acme");

      const watcher = within(ci()).getByRole("region", { name: "CI Watcher" });
      expect(watcher).toHaveTextContent(
        "crashed, restarted 3 times; the last line it wrote: panic: boom",
      );
      expect(watcher).toHaveTextContent("Code Central restarts it by itself");
      expect(watcher).not.toHaveTextContent("tries again every 2 minutes");
    });

    it("shows the service's refusal on its field", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.click(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      );
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      const repository = within(ci()).getByRole("textbox", {
        name: "GitHub repository",
      });
      expect(within(ci()).getByRole("alert")).toHaveTextContent(
        "CI Tracking needs the GitHub repository to watch, as owner/name",
      );
      expect(repository).toHaveAttribute("aria-invalid", "true");
      expect(repository).toHaveAccessibleDescription(
        /needs the GitHub repository/,
      );
      expect((await w.api.listProjects())[0].ci.tracking).toBe(false);

      await userEvent.type(repository, "acme/web");
      expect(within(ci()).queryByRole("alert")).toBeNull();
      const ignored = within(ci()).getByRole("textbox", {
        name: "Ignored workflows",
      });
      await userEvent.type(ignored, ".github/workflows/ci.yml");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(within(ci()).getByRole("alert")).toHaveTextContent(
        "isn't a workflow file name, such as ci.yml",
      );
      expect(ignored).toHaveAttribute("aria-invalid", "true");
    });

    it("shows a refused turn-on's reason and its fix", async () => {
      const { w } = await acme();
      w.setCICheck({
        problem: "logged_out",
        reason: "gh isn't logged in to GitHub.",
        fix: "Run gh auth login as the user the service runs as.",
      });
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.click(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      );
      await userEvent.type(
        within(ci()).getByRole("textbox", { name: "GitHub repository" }),
        "acme/web",
      );
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      const alert = within(ci()).getByRole("alert");
      expect(alert).toHaveTextContent("gh isn't logged in to GitHub.");
      expect(alert).toHaveTextContent(
        "Run gh auth login as the user the service runs as.",
      );
      expect(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      ).toHaveAccessibleDescription(/gh isn't logged in/);
      expect((await w.api.listProjects())[0].ci.tracking).toBe(false);

      w.setCICheck();
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(within(ci()).getByRole("status")).toHaveTextContent("Saved");
      expect(within(ci()).queryByRole("alert")).toBeNull();
      expect((await w.api.listProjects())[0].ci.tracking).toBe(true);
    });

    it("shows a change made elsewhere, as another tab would", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      expect(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      ).not.toBeChecked();

      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: ["nightly.yml"],
      });
      await settle(w);
      expect(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      ).toBeChecked();
      expect(
        within(ci()).getByRole("textbox", { name: "GitHub repository" }),
      ).toHaveValue("acme/web");
      expect(
        within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
      ).toHaveValue("nightly.yml");
    });
  });

  describe("the CI Rules", () => {
    it("lists the rules, each with its Spec and the State its Tickets open in", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      const group = screen.getByRole("group", { name: "CI Rules" });
      expect(ci()).toContainElement(group);
      expect(within(group).getByText("No CI Rules yet.")).toBeInTheDocument();

      await w.api.addCIRule("acme", {
        workflow: "ci.yml",
        branch: "main",
        state: "Ready",
      });
      await w.api.addCIRule("acme", {
        workflow: "any",
        branch: "release",
        state: "Draft",
      });
      await settle(w);
      const list = within(rules()).getByRole("list", {
        name: "CI Rules",
      });
      expect(
        within(list)
          .getAllByRole("listitem")
          .map((li) => li.textContent),
      ).toEqual([
        expect.stringMatching(
          /^ci\.yml on main.*opens Ready Tickets on acme-1/,
        ),
        expect.stringMatching(
          /^any workflow on release.*opens Draft Tickets on acme-2/,
        ),
      ]);
      expect(within(rules()).queryByText("No CI Rules yet.")).toBeNull();
    });

    it("adds a rule, creating its Spec", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");

      const add = within(addForm()).getByRole("button", { name: "Add rule" });
      expect(add).toBeDisabled();
      await userEvent.type(
        within(addForm()).getByRole("textbox", { name: "Workflow" }),
        "ci.yml",
      );
      await userEvent.type(
        within(addForm()).getByRole("textbox", { name: "Branch" }),
        "main",
      );
      await userEvent.selectOptions(
        within(addForm()).getByRole("combobox", { name: "Tickets open as" }),
        "Ready",
      );
      await userEvent.click(add);
      await settle(w);

      expect(
        within(rules()).getByText(/opens Ready Tickets on acme-1/),
      ).toBeInTheDocument();
      expect((await w.api.listProjects())[0].ci.rules).toMatchObject([
        { spec: "acme-1", workflow: "ci.yml", branch: "main", state: "Ready" },
      ]);
      expect((await w.api.getSpec("acme", "acme-1")).title).toBe(
        "CI: ci.yml on main",
      );
      // The form is cleared for the next rule.
      expect(
        within(addForm()).getByRole("textbox", { name: "Workflow" }),
      ).toHaveValue("");
      expect(
        within(addForm()).getByRole("textbox", { name: "Branch" }),
      ).toHaveValue("");
      expect(
        within(addForm()).getByRole("combobox", { name: "Tickets open as" }),
      ).toHaveValue("Draft");
    });

    it("shows a refusal on the field it names", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.type(
        within(addForm()).getByRole("textbox", { name: "Workflow" }),
        "ci.yml",
      );
      const branch = within(addForm()).getByRole("textbox", { name: "Branch" });
      await userEvent.type(branch, "release/*");
      await userEvent.click(
        within(addForm()).getByRole("button", { name: "Add rule" }),
      );
      await settle(w);
      expect(within(addForm()).getByRole("alert")).toHaveTextContent(
        "a CI Rule's branch takes no globs",
      );
      expect(branch).toHaveAttribute("aria-invalid", "true");
      expect((await w.api.listProjects())[0].ci.rules).toEqual([]);

      await userEvent.clear(branch);
      expect(within(addForm()).queryByRole("alert")).toBeNull();
    });

    it("edits a rule in place, or cancels the edit", async () => {
      const { w } = await acme();
      await w.api.addCIRule("acme", {
        workflow: "ci.yml",
        branch: "main",
        state: "Draft",
      });
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.click(
        within(rules()).getByRole("button", { name: "Edit ci.yml on main" }),
      );
      const form = within(rules()).getByRole("form", {
        name: "Edit ci.yml on main",
      });
      const workflow = within(form).getByRole("textbox", { name: "Workflow" });
      expect(workflow).toHaveValue("ci.yml");
      await userEvent.clear(workflow);
      await userEvent.type(workflow, "any");
      await userEvent.click(
        within(form).getByRole("button", { name: "Cancel" }),
      );
      expect(
        within(rules()).queryByRole("form", { name: "Edit ci.yml on main" }),
      ).toBeNull();
      expect((await w.api.listProjects())[0].ci.rules[0].workflow).toBe(
        "ci.yml",
      );

      await userEvent.click(
        within(rules()).getByRole("button", { name: "Edit ci.yml on main" }),
      );
      const again = within(rules()).getByRole("form", {
        name: "Edit ci.yml on main",
      });
      expect(
        within(again).getByRole("textbox", { name: "Workflow" }),
      ).toHaveValue("ci.yml");
      await userEvent.clear(
        within(again).getByRole("textbox", { name: "Branch" }),
      );
      await userEvent.type(
        within(again).getByRole("textbox", { name: "Branch" }),
        "develop",
      );
      await userEvent.selectOptions(
        within(again).getByRole("combobox", { name: "Tickets open as" }),
        "Ready",
      );
      await userEvent.click(
        within(again).getByRole("button", { name: "Save" }),
      );
      await settle(w);

      expect(
        within(rules()).getByText(/opens Ready Tickets on acme-1/),
      ).toHaveTextContent(/^ci\.yml on develop/);
      expect(within(rules()).queryByRole("form", { name: /^Edit/ })).toBeNull();
      expect((await w.api.listProjects())[0].ci.rules).toMatchObject([
        {
          spec: "acme-1",
          workflow: "ci.yml",
          branch: "develop",
          state: "Ready",
        },
      ]);
    });

    it("removes a rule, keeping its Spec as the record", async () => {
      const { w } = await acme();
      await w.api.addCIRule("acme", {
        workflow: "ci.yml",
        branch: "main",
        state: "Draft",
      });
      await renderApp(w, "/?view=project-options&project=acme");

      const remove = within(rules()).getByRole("button", {
        name: "Remove ci.yml on main",
      });
      expect(remove).toHaveAccessibleDescription(/keeps its Spec/);
      await userEvent.click(remove);
      await settle(w);
      expect(
        within(rules()).getByText("No CI Rules yet."),
      ).toBeInTheDocument();
      expect((await w.api.listProjects())[0].ci.rules).toEqual([]);
      expect((await w.api.getSpec("acme", "acme-1")).title).toBe(
        "CI: ci.yml on main",
      );
    });

    it("can't be changed while the Project is archived", async () => {
      const { w, s } = await acme();
      await w.api.addCIRule("acme", {
        workflow: "ci.yml",
        branch: "main",
        state: "Draft",
      });
      await s.project("old", "Old");
      await s.archive("old");
      await renderApp(w, "/?view=project-options&project=old");
      expect(
        within(addForm()).getByRole("textbox", { name: "Workflow" }),
      ).toBeDisabled();
    });
  });

  it("notes the pull requests left open after a switch away from Pull Request, with links", async () => {
    const { w, s } = await acme();
    const id = await s.ticket("acme", { title: "Login form", state: "Accepted" });
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
    await s.reportPushedCommit("acme", id, "abc123", id);
    await s.reportPullRequest("acme", { branch: id, number: 7, link: "https://github.com/acme/web/pull/7", state: "open" });
    await renderApp(w, "/?view=project-options&project=acme");
    expect(within(landing()).queryByRole("status", { name: "Open pull requests" })).toBeNull();

    await userEvent.click(within(landing()).getByRole("radio", { name: "Merge" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    const notice = within(landing()).getByRole("status", { name: "Open pull requests" });
    expect(notice).toHaveTextContent("1 pull request is still open on the Code Host.");
    expect(within(notice).getByRole("link", { name: "https://github.com/acme/web/pull/7" })).toHaveAttribute(
      "href",
      "https://github.com/acme/web/pull/7",
    );
  });

  it("has a section menu whose entries jump to their sections", async () => {
    const { w } = await acme();
    await renderApp(w, "/?view=project-options&project=acme");

    const menu = within(page()).getByRole("navigation", { name: "Sections" });
    const links = within(menu).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual([
      "Repositories",
      "Landing Method",
      "CI",
      "GitHub Issues",
      "Models",
      "Stage Guides",
      "Auto-accept",
      "Focus",
    ]);

    const scrolled: string[] = [];
    Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
      scrolled.push(this.id);
    });
    for (const link of links) {
      const id = link.getAttribute("href")!.slice(1);
      const section = page().querySelector(`#${id}`);
      expect(section).not.toBeNull();
      expect(section!.querySelector("legend")!.textContent).toBe(
        link.textContent,
      );
      await userEvent.click(link);
      expect(scrolled.pop()).toBe(id);
    }
    expect(window.location.search).toBe("?view=project-options&project=acme");
  });
});
