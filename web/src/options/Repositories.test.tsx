// The Repositories section of Project Options (Spec code-central-848): the
// Home Repository, always there and named by the Project Key, then the
// Repositories the human adds, in landing order, each moved up and down,
// edited in place and removed, with every refusal's reason and fix beside
// it. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const page = () => screen.getByRole("main", { name: "Project Options" });
const section = () => within(page()).getByRole("group", { name: "Repositories" });
const list = () => within(section()).getByRole("list", { name: "Repositories" });
const row = (name: string) => within(list()).getByRole("listitem", { name });
const order = () => within(list()).getAllByRole("listitem").map((li) => li.getAttribute("aria-label"));
const addForm = () => within(section()).getByRole("form", { name: "Add a Repository" });

const fresh = async (opts: { archived?: boolean; repositories?: string[] } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  for (const name of opts.repositories ?? []) {
    await w.api.addRepository("acme", { name, clone_url: `git@github.com:acme/${name}.git` });
  }
  if (opts.archived) await s.archive("acme");
  return { w, s };
};

const add = async (w: World, fields: Record<string, string>) => {
  for (const [label, value] of Object.entries(fields)) {
    const input = within(addForm()).getByRole("textbox", { name: label });
    await userEvent.clear(input);
    if (value) await userEvent.type(input, value);
  }
  await userEvent.click(within(addForm()).getByRole("button", { name: "Add Repository" }));
  await settle(w);
};

describe("the Repositories section", () => {
  it("comes first and lists the Home Repository alone, named by the Project Key", async () => {
    const { w } = await fresh();
    await renderApp(w);

    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections[0]).toBe("Repositories");
    expect(within(page()).getByRole("navigation", { name: "Sections" })).toHaveTextContent(/^Repositories/);
    expect(order()).toEqual(["acme"]);
    const home = row("acme");
    expect(home).toHaveTextContent("Home");
    expect(home).toHaveTextContent("Not reported yet");
    const toLanding = within(home).getByRole("link", { name: "Default Landing" });
    expect(toLanding).toHaveAttribute("href", "#project-options-default-landing");
    const scrolled: string[] = [];
    Element.prototype.scrollIntoView = vi.fn(function (this: Element) {
      scrolled.push(this.id);
    });
    await userEvent.click(toLanding);
    expect(scrolled).toEqual(["project-options-default-landing"]);
    expect(window.location.search).toBe("?view=project-options&project=acme");
    expect(within(home).getByRole("button", { name: "Edit acme" })).toBeEnabled();
    expect(within(home).getByRole("button", { name: "Move acme up" })).toBeDisabled();
    expect(within(home).getByRole("button", { name: "Move acme down" })).toBeDisabled();
    const remove = within(home).getByRole("button", { name: "Remove acme" });
    expect(remove).toBeDisabled();
    expect(remove).toHaveAccessibleDescription(/Home Repository can't be removed/);
  });

  it("shows the Home Repository's reported Clone URL and the Project's Base", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await renderApp(w);

    const home = row("acme");
    expect(home).toHaveTextContent("git@github.com:acme/acme.git");
    expect(home).toHaveTextContent("GitHub");
    expect(home).toHaveTextContent("main");
  });

  it("adds a Repository with its Base main by default, at the end of the landing order", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(within(addForm()).getByRole("textbox", { name: "Base" })).toHaveValue("main");
    expect(within(addForm()).getByRole("button", { name: "Add Repository" })).toBeDisabled();
    await add(w, { Name: "web", "Clone URL": "https://dev.azure.com/acme/shop/_git/web", "Local path": "/src/web" });

    expect(order()).toEqual(["acme", "web"]);
    expect(row("web")).toHaveTextContent("https://dev.azure.com/acme/shop/_git/web");
    expect(row("web")).toHaveTextContent("Azure Repos");
    expect(row("web")).toHaveTextContent("/src/web");
    expect(within(addForm()).getByRole("textbox", { name: "Name" })).toHaveValue("");
    expect((await w.api.listProjects())[0].repositories[1]).toMatchObject({ name: "web", base: "main", local_path: "/src/web" });
  });

  it("adds a local-only Repository with a local path and no Clone URL", async () => {
    const { w } = await fresh();
    await renderApp(w);

    const submit = () => within(addForm()).getByRole("button", { name: "Add Repository" });
    const type = (label: string, value: string) => userEvent.type(within(addForm()).getByRole("textbox", { name: label }), value);
    expect(submit()).toBeDisabled();
    await type("Name", "docs");
    expect(submit()).toBeDisabled();
    await type("Local path", "/src/docs");
    expect(submit()).toBeEnabled();
    await userEvent.clear(within(addForm()).getByRole("textbox", { name: "Local path" }));
    expect(submit()).toBeDisabled();
    await type("Local path", "/src/docs");
    await userEvent.click(submit());
    await settle(w);

    expect(order()).toEqual(["acme", "docs"]);
    expect(row("docs")).toHaveTextContent("local-only: never cloned or pushed");
    expect(row("docs")).toHaveTextContent("/src/docs");
    expect(within(row("docs")).getByText("Code Host").nextElementSibling).toBeEmptyDOMElement();
    expect((await w.api.listProjects())[0].repositories[1]).toMatchObject({ name: "docs", clone_url: "", code_host: "", local_path: "/src/docs" });
  });

  it("hints how to leave the Clone URL empty for a local-only Repository", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(within(addForm()).getByRole("textbox", { name: "Clone URL" })).toHaveAccessibleDescription(
      "Leave empty for a local-only repository: give its local path. It is never cloned or pushed.",
    );
  });

  it("offers to watch a Repository's CI only while a Clone URL is entered", async () => {
    const { w, s } = await fresh();
    await s.trackCI("acme");
    await renderApp(w);

    const watch = () => within(addForm()).queryByRole("checkbox", { name: /Watch its CI/ });
    expect(watch()).toBeNull();
    await userEvent.type(within(addForm()).getByRole("textbox", { name: "Clone URL" }), "git@github.com:acme/web.git");
    expect(watch()).not.toBeNull();
    await userEvent.clear(within(addForm()).getByRole("textbox", { name: "Clone URL" }));
    expect(watch()).toBeNull();
  });

  it("clears a Clone URL when the edit form's field is emptied", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await w.api.editRepository("acme", "web", { local_path: "/src/web" });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    const form = within(section()).getByRole("form", { name: "Edit web" });
    await userEvent.clear(within(form).getByRole("textbox", { name: "Clone URL" }));
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(row("web")).toHaveTextContent("local-only: never cloned or pushed");
    expect((await w.api.listProjects())[0].repositories[1]).toMatchObject({ name: "web", clone_url: "", local_path: "/src/web" });
  });

  it("keeps Save disabled once an edit leaves neither a Clone URL nor a local path", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    const form = within(section()).getByRole("form", { name: "Edit web" });
    await userEvent.clear(within(form).getByRole("textbox", { name: "Clone URL" }));
    expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows an add's refusal with its reason and fix, on the field it is about", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await renderApp(w);

    await add(w, { Name: "site", "Clone URL": "https://github.com/acme/web" });
    const alert = within(addForm()).getByRole("alert");
    expect(alert).toHaveTextContent("already listed as Repository web");
    expect(alert).toHaveTextContent("edit web instead");
    expect(within(addForm()).getByRole("textbox", { name: "Clone URL" })).toHaveAttribute("aria-invalid", "true");
    expect(order()).toEqual(["acme", "web"]);
  });

  it("edits a Repository's Clone URL, Base and local path in place, never its name", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    const form = within(section()).getByRole("form", { name: "Edit web" });
    expect(within(form).queryByRole("textbox", { name: "Name" })).toBeNull();
    const base = within(form).getByRole("textbox", { name: "Base" });
    await userEvent.clear(base);
    await userEvent.type(base, "develop");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(within(section()).queryByRole("form", { name: "Edit web" })).toBeNull();
    expect(row("web")).toHaveTextContent("develop");
    expect((await w.api.listProjects())[0].repositories[1]).toMatchObject({ name: "web", base: "develop", clone_url: "git@github.com:acme/web.git" });
  });

  it("keeps an edit open with its refusal, and Cancel leaves the Repository as it was", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    const form = within(section()).getByRole("form", { name: "Edit web" });
    await userEvent.clear(within(form).getByRole("textbox", { name: "Base" }));
    await userEvent.type(within(form).getByRole("textbox", { name: "Base" }), "a..b");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(form).getByRole("alert")).toHaveTextContent("such as main");

    await userEvent.click(within(form).getByRole("button", { name: "Cancel" }));
    expect(row("web")).toHaveTextContent("main");
  });

  it("moves Repositories up and down the landing order", async () => {
    const { w } = await fresh({ repositories: ["web", "api"] });
    await renderApp(w);

    expect(within(row("api")).getByRole("button", { name: "Move api down" })).toBeDisabled();
    await userEvent.click(within(row("api")).getByRole("button", { name: "Move api up" }));
    await settle(w);
    expect(order()).toEqual(["acme", "api", "web"]);

    await userEvent.click(within(row("acme")).getByRole("button", { name: "Move acme down" }));
    await settle(w);
    expect(order()).toEqual(["api", "acme", "web"]);
    expect((await w.api.listProjects())[0].repositories.map((r) => r.name)).toEqual(["api", "acme", "web"]);
  });

  it("removes a Repository", async () => {
    const { w } = await fresh({ repositories: ["web", "api"] });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Remove web" }));
    await settle(w);
    expect(order()).toEqual(["acme", "api"]);
  });

  it("shows each Repository's Landing, marked Project default or its own, its Push remote and its CI choice", async () => {
    const { w, s } = await fresh({ repositories: ["web"] });
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await w.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await w.api.editRepository("acme", "web", { landing: "pull_request", push_remote: "upstream" });
    await w.api.setRepositoryCI("acme", "web", { not_watched: true });
    await renderApp(w);

    const detail = (name: string, term: string) => within(row(name)).getByText(term, { selector: "dt" }).nextElementSibling;
    expect(detail("acme", "Landing")).toHaveTextContent("Merge + Push (Project default)");
    expect(detail("acme", "Push remote")).toHaveTextContent("origin (Project's)");
    expect(detail("acme", "CI")).toHaveTextContent("GitHub Actions on acme/acme");
    expect(detail("web", "Landing")).toHaveTextContent("Pull Request (its own)");
    expect(detail("web", "Push remote")).toHaveTextContent("upstream (its own)");
    expect(detail("web", "CI")).toHaveTextContent("Not watched");
  });

  it("names the Default Landing in Project default as the service names it", async () => {
    const { w, s } = await fresh({ repositories: ["web"] });
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await renderApp(w);
    const landingOf = () => within(row("web")).getByText("Landing", { selector: "dt" }).nextElementSibling;

    await w.api.setLandingMethod("acme", { method: "branch", push: true, remote: "origin" });
    await settle(w);
    expect(landingOf()).toHaveTextContent("Branch + Push (Project default)");

    // Under Pull Request, Push is implied.
    await w.api.setLandingMethod("acme", { method: "pull_request", push: true, remote: "origin" });
    await settle(w);
    expect(landingOf()).toHaveTextContent("Pull Request (Project default)");
    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    const form = within(section()).getByRole("form", { name: "Edit web" });
    expect(within(form).getByRole("option", { name: "Project default (Pull Request)" })).toBeInTheDocument();
  });

  it("shows a Home whose clone has no Push remote as local-only, Not watched", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "");
    await renderApp(w);

    const detail = (term: string) => within(row("acme")).getByText(term, { selector: "dt" }).nextElementSibling;
    expect(detail("Clone URL")).toHaveTextContent("local-only: its clone has no Push remote");
    expect(detail("Code Host")).toBeEmptyDOMElement();
    expect(detail("CI")).toHaveTextContent("Not watched");
  });

  it("gives a Repository its own Landing and Push remote in place, and sets it back to Project default", async () => {
    const { w } = await fresh({ repositories: ["web"] });
    await renderApp(w);

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    let form = within(section()).getByRole("form", { name: "Edit web" });
    const landing = within(form).getByRole("combobox", { name: "Landing" });
    expect(landing).toHaveValue("default");
    expect(within(landing).getAllByRole("option").map((o) => o.textContent)).toEqual([
      "Project default (Merge)",
      "Merge",
      "Merge + Push",
      "Branch",
      "Branch + Push",
      "Pull Request",
    ]);
    expect(within(form).getByRole("textbox", { name: "Push remote" })).toHaveValue("");
    expect(within(form).getByRole("textbox", { name: "Push remote" })).toHaveAttribute("placeholder", "origin");
    await userEvent.selectOptions(landing, "Branch + Push");
    await userEvent.type(within(form).getByRole("textbox", { name: "Push remote" }), "upstream");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(row("web")).toHaveTextContent("Branch + Push (its own)");
    expect(row("web")).toHaveTextContent("upstream (its own)");
    expect((await w.api.listProjects())[0].repositories[1]).toMatchObject({ landing: "branch_push", own_landing: true, push_remote: "upstream" });

    await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
    form = within(section()).getByRole("form", { name: "Edit web" });
    expect(within(form).getByRole("combobox", { name: "Landing" })).toHaveValue("branch_push");
    await userEvent.selectOptions(within(form).getByRole("combobox", { name: "Landing" }), "Project default (Merge)");
    await userEvent.clear(within(form).getByRole("textbox", { name: "Push remote" }));
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(row("web")).toHaveTextContent("Merge (Project default)");
    expect(row("web")).toHaveTextContent("origin (Project's)");
  });

  it("edits the Home's Landing and Push remote, never its Clone URL, Base or local path", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await renderApp(w);

    await userEvent.click(within(row("acme")).getByRole("button", { name: "Edit acme" }));
    const form = within(section()).getByRole("form", { name: "Edit acme" });
    expect(within(form).queryByRole("textbox", { name: "Clone URL" })).toBeNull();
    expect(within(form).queryByRole("textbox", { name: "Base" })).toBeNull();
    expect(within(form).queryByRole("textbox", { name: "Local path" })).toBeNull();
    await userEvent.selectOptions(within(form).getByRole("combobox", { name: "Landing" }), "Merge + Push");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(row("acme")).toHaveTextContent("Merge + Push (its own)");
    expect(row("acme")).toHaveTextContent("git@github.com:acme/acme.git");
  });

  it("shows a refused Own Landing inline with its fix, keeping the edit open", async () => {
    const { w } = await fresh();
    await w.api.addRepository("acme", { name: "docs", local_path: "/src/docs" });
    await renderApp(w);

    await userEvent.click(within(row("docs")).getByRole("button", { name: "Edit docs" }));
    const form = within(section()).getByRole("form", { name: "Edit docs" });
    await userEvent.selectOptions(within(form).getByRole("combobox", { name: "Landing" }), "Pull Request");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    const alert = within(form).getByRole("alert");
    expect(alert).toHaveTextContent("docs");
    expect(alert).toHaveTextContent("Merge");
    expect(within(form).getByRole("combobox", { name: "Landing" })).toHaveAttribute("aria-invalid", "true");
    expect((await w.api.listProjects())[0].repositories[1].own_landing).toBe(false);
  });

  it("sets the Home's CI choice in its row, like any other Repository's", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await renderApp(w);

    await userEvent.click(within(row("acme")).getByRole("button", { name: "Edit acme's CI" }));
    const form = within(row("acme")).getByRole("form", { name: "acme's CI" });
    expect(within(form).getByRole("textbox", { name: "GitHub repository" })).toHaveValue("acme/acme");
    await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "Not watched");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);

    expect(within(row("acme")).queryByRole("form", { name: "acme's CI" })).toBeNull();
    expect(row("acme")).toHaveTextContent("Not watched");
    expect((await w.api.listProjects())[0].repositories[0].ci).toEqual({ name: "acme", not_watched: true });
  });

  it("flags a local-only Home under Pull Request in its row, with the fix", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await w.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
    await s.reportPushRemote("acme", "");
    await renderApp(w);

    const flag = within(row("acme")).getByRole("alert");
    expect(flag).toHaveTextContent("local-only under Pull Request: nothing lands in acme");
    expect(flag).toHaveTextContent("Give acme an Own Landing such as Merge, or add a Push remote to its clone and restart the Factory.");
  });

  it("notes that Push pushes nothing while every Repository is local-only, and still allows it", async () => {
    const { w, s } = await fresh();
    await s.reportPushRemote("acme", "");
    await w.api.addRepository("acme", { name: "docs", local_path: "/src/docs" });
    await renderApp(w);
    const note = () => within(section()).queryByRole("note");
    expect(note()).toBeNull();

    await w.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await settle(w);
    expect(note()).toHaveTextContent("No Repository has a Clone URL, so Push pushes nothing.");

    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git" });
    await settle(w);
    expect(note()).toBeNull();
  });

  it("is disabled while the Project is archived", async () => {
    const { w } = await fresh({ archived: true });
    await renderApp(w);

    expect(section()).toBeDisabled();
  });

  describe("Check Command", () => {
    const detail = (name: string) =>
      within(row(name)).getByText("Check Command", { selector: "dt" }).nextElementSibling;

    it("saves the Project's from its field at the top, with its help text", async () => {
      const { w } = await fresh();
      await renderApp(w);

      const form = within(section()).getByRole("form", { name: "Check Command" });
      expect(form).toHaveTextContent("Today only Conflict Claims run it");
      expect(within(form).getByRole("button", { name: "Save" })).toBeDisabled();
      await userEvent.type(within(form).getByRole("textbox", { name: "Check Command" }), "make typecheck");
      await userEvent.click(within(form).getByRole("button", { name: "Save" }));
      await settle(w);

      expect((await w.api.listProjects())[0].check_command).toBe("make typecheck");
      expect(within(form).getByRole("status")).toHaveTextContent("Saved");
    });

    it("shows each row's as its own, the Project's, or following the Repository's docs", async () => {
      const { w } = await fresh({ repositories: ["web", "api"] });
      await w.api.editRepository("acme", "web", { check_command: "npm run check" });
      await renderApp(w);

      expect(detail("acme")).toHaveTextContent("none: follows the Repository's own docs");
      expect(detail("web")).toHaveTextContent("npm run check (its own)");

      await w.api.setCheckCommand("acme", "make check");
      await settle(w);
      expect(detail("acme")).toHaveTextContent("make check (Project's)");
      expect(detail("api")).toHaveTextContent("make check (Project's)");
      expect(detail("web")).toHaveTextContent("npm run check (its own)");
    });

    it("gives a Repository its own in place, with the Project's as its placeholder, and sets it back", async () => {
      const { w } = await fresh({ repositories: ["web"] });
      await w.api.setCheckCommand("acme", "make check");
      await renderApp(w);

      await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
      let field = within(within(section()).getByRole("form", { name: "Edit web" })).getByRole("textbox", { name: "Check Command" });
      expect(field).toHaveValue("");
      expect(field).toHaveAttribute("placeholder", "make check");
      await userEvent.type(field, "npm run check");
      await userEvent.click(within(within(section()).getByRole("form", { name: "Edit web" })).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(detail("web")).toHaveTextContent("npm run check (its own)");

      await userEvent.click(within(row("web")).getByRole("button", { name: "Edit web" }));
      field = within(within(section()).getByRole("form", { name: "Edit web" })).getByRole("textbox", { name: "Check Command" });
      expect(field).toHaveValue("npm run check");
      await userEvent.clear(field);
      await userEvent.click(within(within(section()).getByRole("form", { name: "Edit web" })).getByRole("button", { name: "Save" }));
      await settle(w);
      expect(detail("web")).toHaveTextContent("make check (Project's)");
    });

    it("gives the Home its own too", async () => {
      const { w } = await fresh();
      await renderApp(w);

      await userEvent.click(within(row("acme")).getByRole("button", { name: "Edit acme" }));
      const form = within(section()).getByRole("form", { name: "Edit acme" });
      await userEvent.type(within(form).getByRole("textbox", { name: "Check Command" }), "make typecheck");
      await userEvent.click(within(form).getByRole("button", { name: "Save" }));
      await settle(w);

      expect(detail("acme")).toHaveTextContent("make typecheck (its own)");
    });
  });
});
