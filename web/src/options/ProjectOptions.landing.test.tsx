// Project Options: the Default Landing and its Pull Request settings.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { defaultLanding } from "../api/scenario";
import { renderApp, settle } from "../worldTesting";
import { acme, nav, page, landing, ci } from "./ProjectOptions.testutil";

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

  it("shows the PAT variable while any Azure Repos Repository is under Pull Request, by its Own Landing or the default", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.addRepository("acme", { name: "infra", clone_url: "https://dev.azure.com/acme/ops/_git/infra" });
    await renderApp(w, "/?view=project-options&project=acme");
    const pat = () => within(landing()).queryByRole("textbox", { name: "PAT variable" });
    expect(pat()).toBeNull();

    // The default as drafted puts infra under Pull Request.
    await userEvent.click(within(landing()).getByRole("radio", { name: "Pull Request" }));
    expect(pat()).toHaveValue("ADO_PAT");
    await userEvent.click(within(landing()).getByRole("radio", { name: "Merge" }));
    expect(pat()).toBeNull();

    // So does its Own Landing, under a Default Landing of Merge.
    await w.api.editRepository("acme", "infra", { landing: "pull_request" });
    await settle(w);
    expect(pat()).toHaveValue("ADO_PAT");
  });

  it("shows the Pull Request Watcher while any Repository is under Pull Request, its Own Landing included", async () => {
    const { w } = await acme();
    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git" });
    await renderApp(w, "/?view=project-options&project=acme");
    expect(within(landing()).queryByRole("region", { name: "Pull Request Watcher" })).toBeNull();

    await w.api.editRepository("acme", "web", { landing: "pull_request" });
    await settle(w);
    expect(within(landing()).getByRole("region", { name: "Pull Request Watcher" })).toBeInTheDocument();
  });

  it("offers each drafted method's Push, Land per Spec, Rebase, Code Host and PAT parts, and the Watcher as saved", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "https://dev.azure.com/acme/shop/_git/acme");
    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git", landing: "pull_request" });
    await renderApp(w, "/?view=project-options&project=acme");
    const parts = () => ({
      push: within(landing()).queryByRole("checkbox", { name: "Push" }) !== null,
      landPerSpec: within(landing()).getByRole("checkbox", { name: "Land per Spec" }).hasAttribute("disabled") ? "disabled" : "enabled",
      rebase: within(landing()).getByRole("checkbox", { name: "Rebase instead of merging" }).hasAttribute("disabled") ? "disabled" : "enabled",
      codeHost: within(landing()).queryByRole("status", { name: "Code Host" })?.textContent ?? null,
      pat: within(landing()).queryByRole("textbox", { name: "PAT variable" }) !== null,
      watcher: within(landing()).queryByRole("region", { name: "Pull Request Watcher" }) !== null,
    });
    const draft = (name: string) => userEvent.click(within(landing()).getByRole("radio", { name }));

    expect(parts()).toEqual({ push: true, landPerSpec: "enabled", rebase: "enabled", codeHost: null, pat: false, watcher: true });
    await draft("Leave as a branch");
    expect(parts()).toEqual({ push: true, landPerSpec: "disabled", rebase: "disabled", codeHost: null, pat: false, watcher: true });
    await draft("Pull Request");
    expect(parts()).toEqual({
      push: false,
      landPerSpec: "enabled",
      rebase: "enabled",
      codeHost: "Code Host: Azure Repos, repository acme/shop/acme.",
      pat: true,
      watcher: true,
    });

    // The Watcher follows what is saved, not the draft.
    await w.api.editRepository("acme", "web", { landing: "default" });
    await settle(w);
    expect(parts()).toMatchObject({ push: false, pat: true, watcher: false });
    await draft("Merge");
    expect(parts()).toEqual({ push: true, landPerSpec: "enabled", rebase: "enabled", codeHost: null, pat: false, watcher: false });
  });

  it("refuses a Default Landing of Pull Request while a local-only Repository follows it, with a fix naming it", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.addRepository("acme", { name: "docs", local_path: "/src/docs" });
    await renderApp(w, "/?view=project-options&project=acme");

    await userEvent.click(within(landing()).getByRole("radio", { name: "Pull Request" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    const alert = within(landing()).getByRole("alert");
    expect(alert).toHaveTextContent("docs");
    expect(alert).toHaveTextContent("give docs an Own Landing such as Merge");
    expect((await w.api.listProjects())[0].landing.method).toBe("merge");
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

  it("shows Rebase off beside Land per Spec, enabled under Merge and Pull Request, kept across method switches, and saves it", async () => {
    const { w, s } = await acme();
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await renderApp(w, "/?view=project-options&project=acme");

    const box = within(landing()).getByRole("checkbox", { name: "Rebase instead of merging" });
    expect(box).not.toBeChecked();
    expect(box).toBeEnabled();

    await userEvent.click(box);
    await userEvent.click(within(landing()).getByRole("radio", { name: "Leave as a branch" }));
    expect(box).toBeDisabled();
    expect(box).toBeChecked();
    await userEvent.click(within(landing()).getByRole("radio", { name: "Pull Request" }));
    expect(box).toBeEnabled();
    expect(box).toBeChecked();

    await userEvent.click(within(landing()).getByRole("radio", { name: "Merge" }));
    await userEvent.click(within(landing()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.listProjects())[0].landing).toMatchObject({ method: "merge", rebase: true });
  });

  it("shows the current setting, with the Push remote enabled without Push, for the Repositories with Own Landings", async () => {
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
    const remote = within(landing()).getByRole("textbox", { name: "Push remote" });
    expect(push).not.toBeChecked();
    expect(remote).toHaveValue("upstream");
    expect(remote).toBeEnabled();
    expect(remote).toHaveAccessibleDescription(/each Repository with no Push remote of its own/);
    expect(push).toHaveAccessibleDescription(
      /checks the remote exists, never force-pushes, and under Merge pushes the whole base/,
    );
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
      within(landing()).getByRole("textbox", { name: "Push remote" }),
    ).toHaveValue("origin");
    expect(save).toBeDisabled();

    await userEvent.click(
      within(landing()).getByRole("radio", { name: "Leave as a branch" }),
    );
    await userEvent.click(
      within(landing()).getByRole("checkbox", { name: "Push" }),
    );
    const remote = within(landing()).getByRole("textbox", { name: "Push remote" });
    await userEvent.clear(remote);
    await userEvent.click(save);
    await settle(w);
    expect(within(landing()).getByRole("alert")).toHaveTextContent(
      "Push needs a git remote",
    );
    expect(remote).toHaveAccessibleDescription(/Push needs a git remote/);
    expect(remote).toHaveAttribute("aria-invalid", "true");
    expect((await w.api.listProjects())[0].landing).toEqual(defaultLanding());

    await userEvent.type(remote, "upstream");
    expect(within(landing()).queryByRole("alert")).toBeNull();
    await userEvent.click(save);
    await settle(w);
    expect(within(landing()).getByRole("status")).toHaveTextContent("Saved");
    expect((await w.api.listProjects())[0].landing).toEqual(
      defaultLanding({ method: "branch", push: true, remote: "upstream", repository_landing: "branch_push" }),
    );
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

    const remote = within(landing()).getByRole("textbox", { name: "Push remote" });
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
      within(landing()).getByRole("textbox", { name: "Push remote" }),
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
      within(ci()).getByRole("textbox", { name: "Ignored workflows" }),
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
});
