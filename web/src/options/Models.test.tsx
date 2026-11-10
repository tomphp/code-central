// The Models section of Project Options (code-central-310): the Project
// default row, then a row per kind of work, each with a model picker
// (free text, empty for Default) and an effort picker (the valid levels,
// plus Default); Default says what it inherits; a rejected effort shows
// the error. Against FakeApi.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/?view=project-options&project=acme");
  return render(<App api={fake} now={() => fake.now} />);
}

const KINDS = ["grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "implementation"];

const page = () => screen.getByRole("main", { name: "Project Options" });
const models = () => within(page()).getByRole("group", { name: "Models" });
const findModels = async () => within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "Models" });
const model = (row: string) => within(models()).getByRole("combobox", { name: `Model for ${row}` });
const effort = (row: string) => within(models()).getByRole("combobox", { name: `Effort for ${row}` });
const options = (select: HTMLElement) => within(select).getAllByRole("option").map((o) => o.textContent);
const save = () => userEvent.click(within(models()).getByRole("button", { name: "Save" }));
const unset = { model: "", effort: "" };
const fresh = (fake: FakeApi) => {
  fake.addProject("acme", "Acme");
  return fake;
};

describe("the Models section", () => {
  it("comes last, with the Project default row then a row per kind of work, all at Default", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findModels();
    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections).toEqual(["Landing Method", "CI", "GitHub Issues", "Models"]);
    expect(within(models()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["Project default", ...KINDS]);
    for (const row of ["the Project default", ...KINDS]) {
      expect(model(row)).toHaveValue("");
      expect(effort(row)).toHaveValue("");
    }
    expect(within(models()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows what Default inherits: Claude Code's default for the Project default, the Project default for a kind", async () => {
    const fake = fresh(new FakeApi());
    await fake.setModels("acme", { default: { model: "opus", effort: "" }, kinds: [] });
    renderApp(fake);

    await findModels();
    expect(model("the Project default")).toHaveAttribute("placeholder", "Default (Claude Code's default)");
    expect(options(effort("the Project default"))[0]).toBe("Default (Claude Code's default)");
    expect(model("implementation")).toHaveAttribute("placeholder", "Default (opus)");
    expect(options(effort("implementation"))[0]).toBe("Default (Claude Code's default)");

    // It follows the Project default as typed.
    await userEvent.selectOptions(effort("the Project default"), "high");
    expect(options(effort("spec"))[0]).toBe("Default (high)");
  });

  it("offers the valid effort levels after Default", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findModels();
    expect(options(effort("spec")).slice(1)).toEqual(["low", "medium", "high", "xhigh", "max"]);
  });

  it("saves the rows whole, a model as typed and an effort as picked, and back to Default", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findModels();
    await userEvent.type(model("the Project default"), "opus");
    await userEvent.type(model("grilling"), "claude-fable-5-1");
    await userEvent.selectOptions(effort("implementation"), "xhigh");
    await save();

    await within(models()).findByRole("status");
    const stored = (await fake.listProjects())[0].models;
    expect(stored.default).toEqual({ model: "opus", effort: "" });
    expect(stored.kinds.find((k) => k.kind === "grilling")).toEqual({ kind: "grilling", model: "claude-fable-5-1", effort: "" });
    expect(stored.kinds.find((k) => k.kind === "implementation")).toEqual({ kind: "implementation", model: "", effort: "xhigh" });
    expect(within(models()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.clear(model("grilling"));
    await userEvent.selectOptions(effort("implementation"), "");
    await save();
    await within(models()).findByRole("status");
    const cleared = (await fake.listProjects())[0].models;
    expect(cleared.kinds.find((k) => k.kind === "grilling")).toEqual({ kind: "grilling", ...unset });
    expect(cleared.kinds.find((k) => k.kind === "implementation")).toEqual({ kind: "implementation", ...unset });
  });

  it("shows a rejected effort's error on its picker", async () => {
    const fake = fresh(new FakeApi());
    await fake.setModels("acme", { default: unset, kinds: [{ kind: "spec", model: "", effort: "max" }] });
    // A later report dropped max, so saving the row again is refused.
    fake.seedEffortLevels(["low", "medium", "high"]);
    renderApp(fake);

    await findModels();
    expect(effort("spec")).toHaveValue("max");
    await userEvent.type(model("spec"), "opus");
    await save();

    const alert = await within(models()).findByRole("alert");
    expect(alert).toHaveTextContent(`effort "max" isn't a valid level; use one of low, medium, high`);
    expect(effort("spec")).toHaveAttribute("aria-invalid", "true");
    expect(effort("spec")).toHaveAccessibleDescription(/isn't a valid level/);
  });

  it("follows the Project as read, so a change from another tab shows", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findModels();
    await fake.setModels("acme", { default: unset, kinds: [{ kind: "tickets", model: "sonnet", effort: "low" }] });
    expect(await within(models()).findByDisplayValue("sonnet")).toBe(model("tickets"));
    expect(effort("tickets")).toHaveValue("low");
  });

  it("is disabled while the Project is archived", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme", { archived: true });
    renderApp(fake);

    expect(await findModels()).toBeDisabled();
  });
});
