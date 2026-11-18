// Project Options: the CI section.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { SERVICE_LIFE_MS, WORLD_HOST } from "../api/core/core";
import { renderApp, settle } from "../worldTesting";
import { acme, ci, page } from "./ProjectOptions.testutil";

describe("Project Options", () => {
  describe("the CI section", () => {
    it("holds only CI Tracking, off by default, the Ignored workflows and the CI Rules: each Repository's CI is in its row", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");

      const group = screen.getByRole("group", { name: "CI" });
      const tracking = within(group).getByRole("checkbox", {
        name: "CI Tracking",
      });
      expect(tracking).not.toBeChecked();
      expect(tracking).toHaveAccessibleDescription(
        /watches the CI of what the Factory pushes.*set in its row under Repositories/,
      );
      expect(within(group).queryByRole("combobox", { name: "CI Provider" })).toBeNull();
      expect(within(group).queryByRole("textbox", { name: "GitHub repository" })).toBeNull();
      expect(within(group).queryByRole("textbox", { name: "PAT environment variable" })).toBeNull();
      expect(within(group).queryByRole("region", { name: "CI per Repository" })).toBeNull();
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

    it("shows the Home's CI in its row, filled in from the Push remote: GitHub Actions, Azure Pipelines, or Not watched on another host", async () => {
      const { w, s } = await acme();
      await s.project("beta", "Beta");
      await s.project("gamma", "Gamma");
      await s.reportPushRemote("acme", "git@github.com:acme/web.git");
      await s.reportPushRemote("beta", "git@ssh.dev.azure.com:v3/beta/web/app");
      await s.reportPushRemote("gamma", "git@gitlab.com:gamma/web.git");
      const homeCI = () =>
        within(within(page()).getByRole("listitem", { name: /^(acme|beta|gamma)$/ })).getByText("CI", { selector: "dt" }).nextElementSibling;

      const { unmount } = await renderApp(w, "/?view=project-options&project=acme");
      expect(homeCI()).toHaveTextContent("GitHub Actions on acme/web");
      unmount();
      const beta = await renderApp(w, "/?view=project-options&project=beta");
      expect(homeCI()).toHaveTextContent("Azure Pipelines on https://dev.azure.com/beta/web");
      beta.unmount();
      await renderApp(w, "/?view=project-options&project=gamma");
      expect(homeCI()).toHaveTextContent("Not watched");
    });

    // homeCIForm opens the Home Repository acme's CI choice in its row.
    const homeCIForm = async () => {
      const home = within(page()).getByRole("listitem", { name: "acme" });
      await userEvent.click(within(home).getByRole("button", { name: "Edit acme's CI" }));
      return within(home).getByRole("form", { name: "acme's CI" });
    };

    it("shows the PAT environment variable and pipeline names once a Repository's CI is Azure Pipelines, keeping each provider's values in its row", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote("acme", "git@github.com:acme/web.git");
      await renderApp(w, "/?view=project-options&project=acme");
      const group = ci();
      expect(within(group).queryByRole("textbox", { name: "PAT environment variable" })).toBeNull();

      let form = await homeCIForm();
      await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "Azure Pipelines");
      expect(within(form).queryByRole("textbox", { name: "GitHub repository" })).toBeNull();
      await userEvent.type(within(form).getByRole("textbox", { name: "Azure DevOps project URL" }), "https://dev.azure.com/acme/web/_build");
      await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "GitHub Actions");
      expect(within(form).getByRole("textbox", { name: "GitHub repository" })).toHaveValue("acme/web");
      await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "Azure Pipelines");
      await userEvent.click(within(form).getByRole("button", { name: "Save" }));
      await settle(w);

      const pat = within(group).getByRole("textbox", { name: "PAT environment variable" });
      expect(pat).toHaveValue("ADO_PAT");
      expect(pat).toHaveAccessibleDescription(
        /Build: Read & execute, in the service's environment\. Code Central keeps the name, never the PAT\./,
      );
      expect(within(group).getByRole("textbox", { name: "Ignored workflows" })).toHaveAccessibleDescription(/^Pipeline names/);
      await userEvent.clear(pat);
      await userEvent.type(pat, "ACME_PAT");
      await userEvent.click(within(group).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(within(group).getByRole("status")).toHaveTextContent("Saved");
      const p = (await w.api.listProjects())[0];
      expect(p.ci.pat_env).toBe("ACME_PAT");
      expect(p.repositories[0].ci).toMatchObject({ name: "acme", not_watched: false, provider: "azure_pipelines", project_url: "https://dev.azure.com/acme/web" });

      form = await homeCIForm();
      expect(within(form).getByRole("combobox", { name: "CI" })).toHaveValue("azure_pipelines");
    });

    it("shows field errors on the project URL in the row and on the PAT environment variable", async () => {
      const { w } = await acme();
      await renderApp(w, "/?view=project-options&project=acme");
      const form = await homeCIForm();
      await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "Azure Pipelines");
      await userEvent.click(within(form).getByRole("button", { name: "Save" }));
      await settle(w);
      const url = within(form).getByRole("textbox", { name: "Azure DevOps project URL" });
      expect(url).toBeInvalid();
      expect(within(form).getByRole("alert")).toHaveTextContent("CI Tracking needs the Azure DevOps project to watch, as its URL");

      await userEvent.type(url, "https://dev.azure.com/acme/web");
      await userEvent.click(within(form).getByRole("button", { name: "Save" }));
      await settle(w);
      const pat = within(ci()).getByRole("textbox", { name: "PAT environment variable" });
      await userEvent.clear(pat);
      await userEvent.type(pat, "MY-PAT");
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(pat).toBeInvalid();
      expect(within(ci()).getByRole("alert")).toHaveTextContent("isn't an environment variable name");
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

    it("turns CI Tracking on with Ignored workflows, watching the Home as its row names it", async () => {
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
      const p = (await w.api.listProjects())[0];
      expect(p.ci).toMatchObject({
        tracking: true,
        pat_env: "ADO_PAT",
        ignored_workflows: ["nightly.yml", "preview.yml"],
      });
      expect(p.repositories[0].ci).toEqual({ name: "acme", not_watched: false, provider: "github_actions", repository: "acme/web" });
      expect(within(ci()).getByRole("region", { name: "CI Watcher" })).toHaveTextContent("CI Watcher: Starting");
      expect(
        within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
      ).toHaveValue("nightly.yml, preview.yml");
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(tracking);
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await settle(w);
      expect((await w.api.listProjects())[0].ci.tracking).toBe(false);
      expect((await w.api.listProjects())[0].repositories[0].ci.repository).toBe("acme/web");
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

    it("shows the host whose service runs the CI Watcher, or that none does", async () => {
      const { w } = await acme();
      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: [],
      });
      w.holdWatcherLease("acme", WORLD_HOST);
      await renderApp(w, "/?view=project-options&project=acme");
      const watcher = () => within(ci()).getByRole("region", { name: "CI Watcher" });
      expect(watcher()).toHaveTextContent("CI Watcher: Starting; it hasn't reported yet.");
      expect(watcher()).toHaveTextContent("It runs on laptop.");

      w.advance(SERVICE_LIFE_MS);
      w.holdWatcherLease("acme", "desktop");
      await settle(w);
      expect(watcher()).toHaveTextContent(
        "CI Watcher: Runs on desktop, whose service holds this Project's Watchers; its status shows there.",
      );

      // No service renews it, so it lapses; the Project's next change
      // shows that.
      w.advance(SERVICE_LIFE_MS);
      await w.api.setCITracking("acme", {
        tracking: true,
        provider: "github_actions",
        repository: "acme/web",
        ignored_workflows: ["deploy.yml"],
      });
      await settle(w);
      expect(watcher()).toHaveTextContent("CI Watcher: Waiting for a service to take the Watchers.");
      expect(watcher()).not.toHaveTextContent("laptop");
    });

    it("shows the service's refusal on its field", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote("acme", "git@github.com:acme/web.git");
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.click(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
      );
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
      expect(ignored).toHaveAccessibleDescription(/isn't a workflow file name/);
      expect((await w.api.listProjects())[0].ci.tracking).toBe(false);
    });

    it("shows a refused turn-on's reason and its fix", async () => {
      const { w, s } = await acme();
      await s.reportPushRemote("acme", "git@github.com:acme/web.git");
      w.setCICheck({
        problem: "logged_out",
        reason: "gh isn't logged in to GitHub.",
        fix: "Run gh auth login as the user the service runs as.",
      });
      await renderApp(w, "/?view=project-options&project=acme");

      await userEvent.click(
        within(ci()).getByRole("checkbox", { name: "CI Tracking" }),
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
        within(page()).getByRole("listitem", { name: "acme" }),
      ).toHaveTextContent("GitHub Actions on acme/web");
      expect(
        within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
      ).toHaveValue("nightly.yml");
    });
  });
});
