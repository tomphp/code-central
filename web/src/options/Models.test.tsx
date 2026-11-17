// The Models section of Project Options (code-central-310): the Project
// default row, then a row per kind of work, each with a model picker
// (free text, empty for Default, suggesting the Reported Models' aliases
// and model IDs) and an effort picker (the valid levels, plus Default);
// Default says what it inherits; a rejected effort shows the error.
// On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { options, suggested } from "../testPickers";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const KINDS = ["grilling", "spec", "tickets", "map-chart", "map-grilling", "map-research", "map-task", "prototype", "retro", "implementation"];

const page = () => screen.getByRole("main", { name: "Project Options" });
const models = () => within(page()).getByRole("group", { name: "Models" });
// A model picker is a textbox, or a combobox once it suggests models.
const model = (row: string) => within(models()).getByLabelText(`Model for ${row}`);
const effort = (row: string) => within(models()).getByRole("combobox", { name: `Effort for ${row}` });
const report = { effort_levels: [], aliases: [], model_flag: "", effort_flag: "" };
const save = async (w: World) => {
  await userEvent.click(within(models()).getByRole("button", { name: "Save" }));
  await settle(w);
};
const unset = { model: "", effort: "" };
const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return { w, s };
};

describe("the Models section", () => {
  it("comes after GitHub Issues, with the Project default row then a row per kind of work, all at Default", async () => {
    const { w } = await fresh();
    await renderApp(w);

    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections).toEqual(["Repositories", "Default Landing", "CI", "GitHub Issues", "Azure Boards", "Models", "Stage Guides", "Auto-accept", "Focus", "Factories"]);
    expect(within(models()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["Project default", ...KINDS, "Chat"]);
    for (const row of ["the Project default", ...KINDS, "Chat"]) {
      expect(model(row)).toHaveValue("");
      expect(effort(row)).toHaveValue("");
    }
    expect(within(models()).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("shows what Default inherits: Claude Code's default for the Project default, the Project default for a kind", async () => {
    const { w } = await fresh();
    await w.api.setModels("acme", { default: { model: "opus", effort: "" }, kinds: [] });
    await renderApp(w);

    expect(model("the Project default")).toHaveAttribute("placeholder", "Default (Claude Code's default)");
    expect(options(effort("the Project default"))[0]).toBe("Default (Claude Code's default)");
    expect(model("implementation")).toHaveAttribute("placeholder", "Default (opus)");
    expect(options(effort("implementation"))[0]).toBe("Default (Claude Code's default)");

    // It follows the Project default as typed.
    await userEvent.selectOptions(effort("the Project default"), "high");
    expect(options(effort("spec"))[0]).toBe("Default (high)");
  });

  it("shows haiku as Chat's model, which neither the Project default nor the Factory's flags reach", async () => {
    const { w, s } = await fresh();
    await w.api.setModels("acme", { default: { model: "opus", effort: "high" }, kinds: [] });
    await s.reportModels("acme", { ...report, model_flag: "sonnet", effort_flag: "low" });
    await renderApp(w);

    expect(model("Chat")).toHaveAttribute("placeholder", "haiku");
    expect(options(effort("Chat"))[0]).toBe("Default (Claude Code's default)");
    // A kind's row still inherits the Project default.
    expect(model("spec")).toHaveAttribute("placeholder", "Default (opus)");
    // Typing a Project default doesn't move Chat's placeholders.
    await userEvent.type(model("the Project default"), "x");
    await userEvent.selectOptions(effort("the Project default"), "max");
    expect(model("Chat")).toHaveAttribute("placeholder", "haiku");
    expect(options(effort("Chat"))[0]).toBe("Default (Claude Code's default)");
  });

  it("sets and reads Chat's model and effort, apart from the kinds of work", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await userEvent.type(model("Chat"), "sonnet");
    await userEvent.selectOptions(effort("Chat"), "low");
    await save(w);

    expect(within(models()).getByRole("status")).toBeInTheDocument();
    const stored = (await w.api.listProjects())[0].models;
    expect(stored.chat).toEqual({ model: "sonnet", effort: "low" });
    expect(stored.default).toEqual(unset);
    expect(stored.kinds.every((k) => k.model === "" && k.effort === "")).toBe(true);
    expect(model("Chat")).toHaveValue("sonnet");

    // A change from another tab shows, and a row back at Default clears.
    await w.api.setModels("acme", { default: unset, chat: { model: "opus", effort: "" }, kinds: [] });
    await settle(w);
    expect(within(models()).getByDisplayValue("opus")).toBe(model("Chat"));
    await userEvent.clear(model("Chat"));
    await save(w);
    expect(within(models()).getByRole("status")).toBeInTheDocument();
    expect((await w.api.listProjects())[0].models.chat).toEqual(unset);
  });

  it("shows a rejected Chat effort's error on Chat's picker", async () => {
    const { w, s } = await fresh();
    await w.api.setModels("acme", { default: unset, chat: { model: "", effort: "max" }, kinds: [] });
    await s.reportModels("acme", { ...report, effort_levels: ["low", "medium", "high"] });
    await renderApp(w);

    await userEvent.type(model("Chat"), "sonnet");
    await save(w);

    expect(within(models()).getByRole("alert")).toBeInTheDocument();
    expect(effort("Chat")).toHaveAttribute("aria-invalid", "true");
  });

  it("offers the valid effort levels after Default", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(options(effort("spec")).slice(1)).toEqual(["low", "medium", "high", "xhigh", "max"]);
  });

  it("offers the effort levels the Factory reported, once it has", async () => {
    const { w, s } = await fresh();
    await s.reportModels("acme", { ...report, effort_levels: ["low", "high", "turbo"] });
    await renderApp(w);

    expect(options(effort("the Project default")).slice(1)).toEqual(["low", "high", "turbo"]);
    expect(options(effort("spec")).slice(1)).toEqual(["low", "high", "turbo"]);
  });

  it("suggests no models before any report: Default and free text only", async () => {
    const { w } = await fresh();
    await renderApp(w);

    for (const row of ["the Project default", ...KINDS, "Chat"]) expect(suggested(model(row))).toEqual([]);
  });

  it("suggests the reported aliases and model IDs in every model picker", async () => {
    const { w, s } = await fresh();
    await s.reportModels("acme", { ...report, aliases: ["fable", "opus", "sonnet"] });
    await s.modelIDs("acme", ["claude-opus-5-5", "opus"]);
    await renderApp(w);

    for (const row of ["the Project default", ...KINDS, "Chat"]) {
      expect(suggested(model(row))).toEqual(["fable", "opus", "sonnet", "claude-opus-5-5"]);
    }
    // Free text still goes.
    await userEvent.type(model("spec"), "my-own-model");
    await save(w);
    expect(within(models()).getByRole("status")).toBeInTheDocument();
    expect((await w.api.listProjects())[0].models.kinds.find((k) => k.kind === "spec")?.model).toBe("my-own-model");
  });

  it("shows the Factory's reported flags as what the Project default's Default inherits", async () => {
    const { w, s } = await fresh();
    await s.reportModels("acme", { ...report, model_flag: "sonnet", effort_flag: "high" });
    await renderApp(w);

    expect(model("the Project default")).toHaveAttribute("placeholder", "Default (Factory flag: sonnet)");
    expect(options(effort("the Project default"))[0]).toBe("Default (Factory flag: high)");
    // A kind's row inherits through the Project default.
    expect(model("spec")).toHaveAttribute("placeholder", "Default (Factory flag: sonnet)");
    await userEvent.type(model("the Project default"), "opus");
    expect(model("spec")).toHaveAttribute("placeholder", "Default (opus)");
  });

  it("saves the rows whole, a model as typed and an effort as picked, and back to Default", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await userEvent.type(model("the Project default"), "opus");
    await userEvent.type(model("grilling"), "claude-fable-5-1");
    await userEvent.selectOptions(effort("implementation"), "xhigh");
    await save(w);

    expect(within(models()).getByRole("status")).toBeInTheDocument();
    const stored = (await w.api.listProjects())[0].models;
    expect(stored.default).toEqual({ model: "opus", effort: "" });
    expect(stored.kinds.find((k) => k.kind === "grilling")).toEqual({ kind: "grilling", model: "claude-fable-5-1", effort: "" });
    expect(stored.kinds.find((k) => k.kind === "implementation")).toEqual({ kind: "implementation", model: "", effort: "xhigh" });
    expect(within(models()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.clear(model("grilling"));
    await userEvent.selectOptions(effort("implementation"), "");
    await save(w);
    expect(within(models()).getByRole("status")).toBeInTheDocument();
    const cleared = (await w.api.listProjects())[0].models;
    expect(cleared.kinds.find((k) => k.kind === "grilling")).toEqual({ kind: "grilling", ...unset });
    expect(cleared.kinds.find((k) => k.kind === "implementation")).toEqual({ kind: "implementation", ...unset });
  });

  it("shows a rejected effort's error on its picker", async () => {
    const { w, s } = await fresh();
    await w.api.setModels("acme", { default: unset, kinds: [{ kind: "spec", model: "", effort: "max" }] });
    // A later report dropped max, so saving the row again is refused.
    await s.reportModels("acme", { ...report, effort_levels: ["low", "medium", "high"] });
    await renderApp(w);

    expect(effort("spec")).toHaveValue("max");
    await userEvent.type(model("spec"), "opus");
    await save(w);

    const alert = within(models()).getByRole("alert");
    expect(alert).toHaveTextContent(`effort "max" isn't a valid level; use one of low, medium, high`);
    expect(effort("spec")).toHaveAttribute("aria-invalid", "true");
    expect(effort("spec")).toHaveAccessibleDescription(/isn't a valid level/);
  });

  it("follows the Project as read, so a change from another tab shows", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await w.api.setModels("acme", { default: unset, kinds: [{ kind: "tickets", model: "sonnet", effort: "low" }] });
    await settle(w);
    expect(within(models()).getByDisplayValue("sonnet")).toBe(model("tickets"));
    expect(effort("tickets")).toHaveValue("low");
  });

  it("is disabled while the Project is archived", async () => {
    const { w } = await fresh({ archived: true });
    await renderApp(w);

    expect(models()).toBeDisabled();
  });
});
