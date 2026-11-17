// Project Options: the CI section.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderApp, settle } from "../worldTesting";
import { acme, ci } from "./ProjectOptions.testutil";

describe("Project Options", () => {
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

});
