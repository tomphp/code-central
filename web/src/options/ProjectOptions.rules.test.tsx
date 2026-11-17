// Project Options: the CI Rules.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { renderApp, settle } from "../worldTesting";
import { acme, ci, rules, addForm } from "./ProjectOptions.testutil";

describe("Project Options", () => {
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

});
