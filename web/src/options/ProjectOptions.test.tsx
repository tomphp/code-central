// The Project Options page: opened from an active Project's menu in the
// left pane, it sets the Project's Landing Method (code-central-174) and
// its CI settings (code-central-251), whose turn-on the CI check may
// refuse (code-central-252). Against FakeApi.
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
      expect(within(group).getByRole("button", { name: "Save" })).toBeDisabled();
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
      await fake.setCITracking("beta", { tracking: false, provider: "github_actions", repository: "beta/fork", ignored_workflows: [] });
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
        push_remote_url: "https://github.com/acme/web.git",
      });
      expect(within(ci()).getByRole("textbox", { name: "Ignored workflows" })).toHaveValue("nightly.yml, preview.yml");
      expect(within(ci()).getByRole("button", { name: "Save" })).toBeDisabled();

      await userEvent.click(tracking);
      await userEvent.click(within(ci()).getByRole("button", { name: "Save" }));
      await waitFor(async () => expect((await fake.listProjects())[0].ci.tracking).toBe(false));
      expect((await fake.listProjects())[0].ci.repository).toBe("acme/web");
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

      await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: ["nightly.yml"] });
      await waitFor(() => expect(within(ci()).getByRole("checkbox", { name: "CI Tracking" })).toBeChecked());
      expect(within(ci()).getByRole("textbox", { name: "GitHub repository" })).toHaveValue("acme/web");
      expect(within(ci()).getByRole("textbox", { name: "Ignored workflows" })).toHaveValue("nightly.yml");
    });
  });
});
