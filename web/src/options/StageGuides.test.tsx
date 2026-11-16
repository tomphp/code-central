// The Stage Guides section of Project Options (code-central-446): the
// Subagent Exploration checkbox, on by default, with what it does and its
// own Save; it follows changes made elsewhere and is read-only on an
// Archived Project. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const guides = () => within(screen.getByRole("main", { name: "Project Options" })).getByRole("group", { name: "Stage Guides" });
const checkbox = (g: HTMLElement) => within(g).getByRole("checkbox", { name: "Subagent Exploration" });
const assignment = (g: HTMLElement) => within(g).getByRole("checkbox", { name: "Model Assignment" });
const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return w;
};

describe("the Stage Guides section", () => {
  it("shows Subagent Exploration on, with what it does and why to turn it off", async () => {
    await renderApp(await fresh());

    const g = guides();
    expect(checkbox(g)).toBeChecked();
    expect(g).toHaveTextContent("Explore subagent");
    expect(g).toHaveTextContent("no subagents or can't afford them");
    expect(g).toHaveTextContent("next Claim");
    for (const save of within(g).getAllByRole("button", { name: "Save" })) expect(save).toBeDisabled();
  });

  it("saves through set_subagent_exploration, leaving the Models rows alone", async () => {
    const w = await fresh();
    await renderApp(w);
    const g = guides();

    await userEvent.click(checkbox(g));
    await userEvent.click(within(g).getAllByRole("button", { name: "Save" })[0]);
    await settle(w);

    expect(within(g).getByRole("status")).toHaveTextContent("Saved");
    expect(w.calls).toContain("setSubagentExploration");
    expect(w.calls).not.toContain("setModels");
    expect((await w.api.listProjects())[0].stage_guides.subagent_exploration).toBe(false);
    expect(checkbox(g)).not.toBeChecked();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const w = await fresh();
    await renderApp(w);
    const g = guides();

    await w.api.setSubagentExploration("acme", false);
    await settle(w);
    expect(checkbox(g)).not.toBeChecked();
  });

  it("is read-only on an Archived Project", async () => {
    await renderApp(await fresh({ archived: true }));

    const g = guides();
    expect(checkbox(g)).toBeDisabled();
    expect(assignment(g)).toBeDisabled();
  });

  it("shows Model Assignment off, with what it does", async () => {
    await renderApp(await fresh());

    const g = guides();
    expect(assignment(g)).not.toBeChecked();
    expect(g).toHaveTextContent("Reported Models");
  });

  it("saves Model Assignment through its own Save, leaving Subagent Exploration alone", async () => {
    const w = await fresh();
    await renderApp(w);
    const g = guides();

    await userEvent.click(assignment(g));
    const saves = within(g).getAllByRole("button", { name: "Save" });
    expect(saves[0]).toBeDisabled();
    await userEvent.click(saves[1]);
    await settle(w);

    expect(within(g).getByRole("status")).toHaveTextContent("Saved");
    expect(w.calls).toContain("setModelAssignment");
    expect(w.calls).not.toContain("setSubagentExploration");
    expect((await w.api.listProjects())[0].stage_guides).toEqual({ subagent_exploration: true, model_assignment: true });
    expect(assignment(g)).toBeChecked();
  });

  it("shows a Model Assignment change made elsewhere, as another tab would", async () => {
    const w = await fresh();
    await renderApp(w);
    const g = guides();

    await w.api.setModelAssignment("acme", true);
    await settle(w);
    expect(assignment(g)).toBeChecked();
  });
});
