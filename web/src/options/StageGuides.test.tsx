// The Stage Guides section of Project Options (code-central-446): the
// Subagent Exploration checkbox, on by default, with what it does and its
// own Save; it follows changes made elsewhere and is read-only on an
// Archived Project. Against FakeApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/?view=project-options&project=acme");
  return render(<App api={fake} now={() => fake.now} />);
}

const guides = async () =>
  within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "Stage Guides" });
const checkbox = (g: HTMLElement) => within(g).getByRole("checkbox", { name: "Subagent Exploration" });
const assignment = (g: HTMLElement) => within(g).getByRole("checkbox", { name: "Model Assignment" });
const fresh = (opts: { archived?: boolean } = {}) => {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme", opts);
  return fake;
};

describe("the Stage Guides section", () => {
  it("shows Subagent Exploration on, with what it does and why to turn it off", async () => {
    renderApp(fresh());

    const g = await guides();
    expect(checkbox(g)).toBeChecked();
    expect(g).toHaveTextContent("Explore subagent");
    expect(g).toHaveTextContent("no subagents or can't afford them");
    expect(g).toHaveTextContent("next Claim");
    for (const save of within(g).getAllByRole("button", { name: "Save" })) expect(save).toBeDisabled();
  });

  it("saves through set_subagent_exploration, leaving the Models rows alone", async () => {
    const fake = fresh();
    renderApp(fake);
    const g = await guides();

    await userEvent.click(checkbox(g));
    await userEvent.click(within(g).getAllByRole("button", { name: "Save" })[0]);

    expect(await within(g).findByRole("status")).toHaveTextContent("Saved");
    expect(fake.calls).toContain("setSubagentExploration");
    expect(fake.calls).not.toContain("setModels");
    expect((await fake.listProjects())[0].stage_guides.subagent_exploration).toBe(false);
    expect(checkbox(g)).not.toBeChecked();
  });

  it("shows a change made elsewhere, as another tab would", async () => {
    const fake = fresh();
    renderApp(fake);
    const g = await guides();

    await fake.setSubagentExploration("acme", false);
    await waitFor(() => expect(checkbox(g)).not.toBeChecked());
  });

  it("is read-only on an Archived Project", async () => {
    renderApp(fresh({ archived: true }));

    const g = await guides();
    expect(checkbox(g)).toBeDisabled();
    expect(assignment(g)).toBeDisabled();
  });

  it("shows Model Assignment off, with what it does", async () => {
    renderApp(fresh());

    const g = await guides();
    expect(assignment(g)).not.toBeChecked();
    expect(g).toHaveTextContent("Reported Models");
  });

  it("saves Model Assignment through its own Save, leaving Subagent Exploration alone", async () => {
    const fake = fresh();
    renderApp(fake);
    const g = await guides();

    await userEvent.click(assignment(g));
    const saves = within(g).getAllByRole("button", { name: "Save" });
    expect(saves[0]).toBeDisabled();
    await userEvent.click(saves[1]);

    expect(await within(g).findByRole("status")).toHaveTextContent("Saved");
    expect(fake.calls).toContain("setModelAssignment");
    expect(fake.calls).not.toContain("setSubagentExploration");
    expect((await fake.listProjects())[0].stage_guides).toEqual({ subagent_exploration: true, model_assignment: true });
    expect(assignment(g)).toBeChecked();
  });

  it("shows a Model Assignment change made elsewhere, as another tab would", async () => {
    const fake = fresh();
    renderApp(fake);
    const g = await guides();

    await fake.setModelAssignment("acme", true);
    await waitFor(() => expect(assignment(g)).toBeChecked());
  });
});
