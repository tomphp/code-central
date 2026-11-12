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
    expect(within(g).getByRole("button", { name: "Save" })).toBeDisabled();
  });

  it("saves through set_subagent_exploration, leaving the Models rows alone", async () => {
    const fake = fresh();
    renderApp(fake);
    const g = await guides();

    await userEvent.click(checkbox(g));
    await userEvent.click(within(g).getByRole("button", { name: "Save" }));

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
  });
});
