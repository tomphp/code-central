// The GitHub Issues section of Project Options (code-central-284): GitHub
// Issues off by default, its repository prefilled from the Push remote,
// the filter label and New issues need me, and a refused turn-on's
// problem and fix in place. Against FakeApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/?view=project-options&project=acme") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const page = () => screen.getByRole("main", { name: "Project Options" });
const issues = () => within(page()).getByRole("group", { name: "GitHub Issues" });
const findIssues = async () => within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "GitHub Issues" });
const field = (name: string) => within(issues()).getByRole("textbox", { name });
const toggle = (name: string) => within(issues()).getByRole("checkbox", { name });
const save = () => userEvent.click(within(issues()).getByRole("button", { name: "Save" }));
const settings = async (fake: FakeApi) => (await fake.listProjects())[0].github_issues;

describe("the GitHub Issues section", () => {
  it("comes after the CI section, off by default with nothing named", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await findIssues();
    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections).toEqual(["Landing Method", "CI", "GitHub Issues"]);

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
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.reportPushRemote("acme", "git@github.com:acme/web.git");
    renderApp(fake);

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
    expect(await settings(fake)).toEqual({ on: true, repository: "acme/tracker", label: "", new_issues_need_me: false, spec: "acme-1" });
    expect(repository).not.toHaveAccessibleDescription(/Push remote/);
  });

  it("turns GitHub Issues on with a filter label and New issues need me, naming the Issues Spec", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.reportPushRemote("acme", "https://github.com/acme/web.git");
    renderApp(fake);

    await findIssues();
    expect(within(issues()).queryByText(/Incoming Issues arrive on/)).toBeNull();
    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("Filter label"), " bug ");
    await userEvent.click(toggle("New issues need me"));
    await save();

    expect(await within(issues()).findByRole("status")).toHaveTextContent("Saved");
    expect(await settings(fake)).toEqual({ on: true, repository: "acme/web", label: "bug", new_issues_need_me: true, spec: "acme-1" });
    expect((await fake.getSpec("acme", "acme-1")).title).toBe("GitHub Issues: acme/web");
    expect(field("Filter label")).toHaveValue("bug");
    expect(within(issues()).getByText(/Incoming Issues arrive on acme-1/)).toBeInTheDocument();
    expect(within(issues()).getByRole("button", { name: "Save" })).toBeDisabled();

    // Turning it off keeps the settings, and the Issues Spec, for later.
    await userEvent.click(toggle("GitHub Issues"));
    await save();
    await waitFor(async () => expect((await settings(fake)).on).toBe(false));
    expect(await settings(fake)).toEqual({ on: false, repository: "acme/web", label: "bug", new_issues_need_me: true, spec: "acme-1" });
  });

  it("shows a refused turn-on's problem and fix in place, leaving GitHub Issues off", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.seedIssuesCheck({
      reason: "The account gh is logged in as can't triage the issues of acme/web.",
      fix: "Give the account gh is logged in as triage permission or more on the repository.",
      field: "repository",
    });
    renderApp(fake);

    await findIssues();
    await userEvent.click(toggle("GitHub Issues"));
    await userEvent.type(field("GitHub repository"), "acme/web");
    await save();
    const alert = await within(issues()).findByRole("alert");
    expect(alert).toHaveTextContent("can't triage the issues of acme/web.");
    expect(alert).toHaveTextContent("Give the account gh is logged in as triage permission");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    expect(field("GitHub repository")).toHaveAccessibleDescription(/can't triage/);
    expect(await settings(fake)).toEqual({ on: false, repository: "", label: "", new_issues_need_me: false });

    fake.seedIssuesCheck({ reason: "gh isn't logged in to GitHub.", fix: "Run gh auth login as the user the service runs as." });
    await save();
    await waitFor(() => expect(within(issues()).getByRole("alert")).toHaveTextContent("gh isn't logged in to GitHub."));
    expect(toggle("GitHub Issues")).toHaveAccessibleDescription(/gh isn't logged in/);
    expect(field("GitHub repository")).not.toHaveAttribute("aria-invalid");

    fake.seedIssuesCheck();
    await save();
    expect(await within(issues()).findByRole("status")).toHaveTextContent("Saved");
    expect(within(issues()).queryByRole("alert")).toBeNull();
    expect((await settings(fake)).on).toBe(true);
  });

  it("shows the service's refusal of a missing repository on its field", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await findIssues();
    await userEvent.click(toggle("GitHub Issues"));
    await save();
    expect(await within(issues()).findByRole("alert")).toHaveTextContent("GitHub Issues needs the GitHub repository its issues come from, as owner/name");
    expect(field("GitHub repository")).toHaveAttribute("aria-invalid", "true");
    await userEvent.type(field("GitHub repository"), "acme/web");
    expect(within(issues()).queryByRole("alert")).toBeNull();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);
    await findIssues();

    await fake.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "triage", new_issues_need_me: true });
    await waitFor(() => expect(toggle("GitHub Issues")).toBeChecked());
    expect(field("GitHub repository")).toHaveValue("acme/web");
    expect(field("Filter label")).toHaveValue("triage");
    expect(toggle("New issues need me")).toBeChecked();
  });

  it("is read-only on an Archived Project", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme", { archived: true });
    renderApp(fake);
    await findIssues();
    expect(toggle("GitHub Issues")).toBeDisabled();
    expect(field("GitHub repository")).toBeDisabled();
  });
});
