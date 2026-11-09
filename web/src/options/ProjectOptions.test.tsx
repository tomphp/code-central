// The Project Options page: opened from an active Project's menu in the
// left pane, it sets the Project's Landing Method (code-central-174).
// Against FakeApi.
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

    const save = await within(page()).findByRole("button", { name: "Save" });
    expect(within(landing()).getByRole("radio", { name: "Merge into base" })).toBeChecked();
    expect(within(landing()).getByRole("textbox", { name: "Remote" })).toHaveValue("origin");
    expect(save).toBeDisabled();

    await userEvent.click(within(landing()).getByRole("radio", { name: "Leave as a branch" }));
    await userEvent.click(within(landing()).getByRole("checkbox", { name: "Push" }));
    const remote = within(landing()).getByRole("textbox", { name: "Remote" });
    await userEvent.clear(remote);
    await userEvent.click(save);
    expect(await within(page()).findByRole("alert")).toHaveTextContent("Push needs a git remote");
    expect(remote).toHaveAccessibleDescription(/Push needs a git remote/);
    expect(remote).toHaveAttribute("aria-invalid", "true");
    expect((await fake.listProjects())[0].landing).toEqual({ method: "merge", push: false, remote: "origin" });

    await userEvent.type(remote, "upstream");
    expect(within(page()).queryByRole("alert")).toBeNull();
    await userEvent.click(save);
    expect(await within(page()).findByRole("status")).toHaveTextContent("Saved");
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
    await userEvent.click(within(page()).getByRole("button", { name: "Save" }));
    expect(await within(page()).findByRole("status")).toHaveTextContent("Saved");
    expect(remote).toHaveValue("upstream");
    expect(within(page()).getByRole("button", { name: "Save" })).toBeDisabled();
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
    expect(within(screen.getByRole("menu", { hidden: true })).queryByRole("menuitem", { name: "Options", hidden: true })).toBeNull();
  });

  it("shows an Archived Project's options read-only, by its URL", async () => {
    const fake = new FakeApi();
    fake.addProject("old", "Old App", { archived: true });
    renderApp(fake, "/?view=project-options&project=old");
    expect(await within(page()).findByText(/archived: unarchive it to change its options/)).toBeInTheDocument();
    expect(within(landing()).getByRole("radio", { name: "Merge into base" })).toBeDisabled();
    expect(within(landing()).getByRole("checkbox", { name: "Push" })).toBeDisabled();
    expect(within(landing()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("says when the URL names no such Project", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake, "/?view=project-options&project=nope");
    expect(await within(page()).findByText("There is no Project nope.")).toBeInTheDocument();
    expect(within(page()).getByRole("link", { name: "Back to the inbox" })).toBeInTheDocument();
  });
});
