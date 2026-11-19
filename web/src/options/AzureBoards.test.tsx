// The Azure Boards section of Project Options (Spec code-central-894): off
// by default, the mode switch, the project URL prefilled from an Azure
// Repos Repository or the Azure Pipelines setting, a refused save's problem
// and fix in place, and the Work Item Watcher's status. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const fresh = async () => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
};

const page = () => screen.getByRole("main", { name: "Project Options" });
const boards = () => within(page()).getByRole("group", { name: "Azure Boards" });
const field = (name: string) => within(boards()).getByRole("textbox", { name });
const toggle = (name: string) => within(boards()).getByRole("checkbox", { name });
const mode = (name: string) => within(boards()).getByRole("radio", { name });
const save = async (w: World) => {
  await userEvent.click(within(boards()).getByRole("button", { name: "Save" }));
  await settle(w);
};
const watcher = () => within(boards()).getByRole("region", { name: "Work Item Watcher" });
const QUERY = "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10";

describe("the Azure Boards section", () => {
  it("comes after GitHub Issues, off by default in Saved query mode", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(toggle("Azure Boards")).not.toBeChecked();
    expect(mode("Saved query")).toBeChecked();
    expect(field("Query URL")).toHaveValue("");
    expect(within(boards()).queryByRole("textbox", { name: "WHERE clause" })).toBeNull();
    expect(toggle("New issues need me")).not.toBeChecked();
    expect(within(boards()).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(within(boards()).queryByRole("region", { name: "Work Item Watcher" })).toBeNull();
  });

  it("shows Hide Code Central off with its help text, and saves it, even while Azure Boards is off", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(toggle("Hide Code Central")).not.toBeChecked();
    expect(within(boards()).getByText(/Write-back still posts every comment, but writes no/)).toHaveTextContent(
      "Write-back still posts every comment, but writes no code-central: tag, no link back and no Spec or Ticket IDs. Tags and comments already on work items stay as they are.",
    );
    await userEvent.click(toggle("Hide Code Central"));
    await save(w);

    expect(toggle("Hide Code Central")).toBeChecked();
    expect((await w.api.listProjects())[0].azure_boards).toMatchObject({ on: false, hide_code_central: true });
  });

  it("shows the Branch format empty with feat/{id} as its example, and saves it", async () => {
    const { w } = await fresh();
    await renderApp(w);

    expect(field("Branch format")).toHaveValue("");
    expect(field("Branch format")).toHaveAttribute("placeholder", "feat/{id}");
    // {{ is user-event's escape for a literal {.
    await userEvent.type(field("Branch format"), "feat/{{id}");
    await save(w);

    expect(within(boards()).getByRole("status")).toHaveTextContent("Saved");
    expect(field("Branch format")).toHaveValue("feat/{id}");
    expect((await w.api.listProjects())[0].azure_boards).toMatchObject({ on: false, branch_format: "feat/{id}" });
  });

  it("shows a refused Branch format in place, against its field, and keeps the format as it was", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await userEvent.type(field("Branch format"), "spec/{{id}");
    await save(w);

    expect(within(boards()).getByRole("alert")).toHaveTextContent("starts with spec/");
    expect(field("Branch format")).toHaveAttribute("aria-invalid", "true");
    expect((await w.api.listProjects())[0].azure_boards.branch_format).toBe("");
  });

  it("switches to a WIQL filter's project URL and WHERE fields, and turns on with them", async () => {
    const { w } = await fresh();
    await renderApp(w);

    await userEvent.click(mode("WIQL filter"));
    expect(within(boards()).queryByRole("textbox", { name: "Query URL" })).toBeNull();
    expect(field("WHERE clause")).toHaveAttribute("placeholder", expect.stringContaining("[System.State]"));
    await userEvent.click(toggle("Azure Boards"));
    await userEvent.type(field("Azure DevOps project URL"), "https://dev.azure.com/contoso/web");
    // [[ is user-event's escape for a literal [.
    await userEvent.type(field("WHERE clause"), "[[System.State] = 'Approved'");
    await userEvent.click(toggle("New issues need me"));
    await save(w);

    expect(within(boards()).getByRole("status")).toHaveTextContent("Saved");
    const a = (await w.api.listProjects())[0].azure_boards;
    expect(a).toMatchObject({ on: true, mode: "wiql", project_url: "https://dev.azure.com/contoso/web", where: "[System.State] = 'Approved'", new_issues_need_me: true });
    expect(within(boards()).getByText(new RegExp(`Incoming Issues arrive on ${a.spec}`))).toBeInTheDocument();
    expect((await w.api.getSpec("acme", a.spec!)).title).toBe("Azure Boards: contoso/web");

    // Back to a Saved query, whose own field kept its value.
    await userEvent.click(mode("Saved query"));
    await userEvent.type(field("Query URL"), QUERY);
    await save(w);
    expect((await w.api.listProjects())[0].azure_boards).toMatchObject({ mode: "saved_query", query_url: QUERY, where: "[System.State] = 'Approved'" });
  });

  it("prefills the project URL from an Azure Repos Repository's Clone URL", async () => {
    const { w, s } = await fresh();
    await s.repository("acme", "api", "https://contoso@dev.azure.com/contoso/web/_git/api");
    await renderApp(w);

    await userEvent.click(mode("WIQL filter"));
    expect(field("Azure DevOps project URL")).toHaveValue("https://dev.azure.com/contoso/web");
    expect(field("Azure DevOps project URL")).toHaveAccessibleDescription(/From this Project's Azure Repos Repository/);
  });

  it("prefills the project URL from the Azure Pipelines CI setting", async () => {
    const { w } = await fresh();
    await w.api.setCITracking("acme", {
      tracking: false,
      provider: "azure_pipelines",
      repository: "",
      project_url: "https://dev.azure.com/fabrikam/shop",
      ignored_workflows: [],
    });
    await renderApp(w);

    await userEvent.click(mode("WIQL filter"));
    expect(field("Azure DevOps project URL")).toHaveValue("https://dev.azure.com/fabrikam/shop");
  });

  it("shows a refused save's problem and fix in place, leaving Azure Boards off", async () => {
    const { w } = await fresh();
    w.setAzureBoardsCheck({
      problem: "pat_missing",
      reason: "The environment variable ADO_PAT isn't set.",
      fix: "Set ADO_PAT to an Azure DevOps PAT in the service's environment.",
    });
    await renderApp(w);

    await userEvent.click(toggle("Azure Boards"));
    await userEvent.type(field("Query URL"), QUERY);
    await save(w);
    const alert = within(boards()).getByRole("alert");
    expect(alert).toHaveTextContent("The environment variable ADO_PAT isn't set.");
    expect(alert).toHaveTextContent("Set ADO_PAT to an Azure DevOps PAT");
    expect((await w.api.listProjects())[0].azure_boards.on).toBe(false);

    w.setAzureBoardsCheck();
    await save(w);
    expect(within(boards()).getByRole("status")).toHaveTextContent("Saved");
    expect(within(boards()).queryByRole("alert")).toBeNull();
    expect((await w.api.listProjects())[0].azure_boards.on).toBe(true);
  });

  it("shows the Work Item Watcher's status while on, live", async () => {
    const { w, s } = await fresh();
    await renderApp(w);
    await w.api.setAzureBoards("acme", { on: true, mode: "saved_query", query_url: QUERY, project_url: "", where: "", pat_env: "", new_issues_need_me: false, hide_code_central: false, branch_format: "" });
    await settle(w);
    expect(watcher()).toHaveTextContent("Work Item Watcher: Starting; it hasn't reported yet.");

    await s.reportWorkItemWatcher("acme", {
      state: "error",
      problem: "query_unusable",
      message: "The saved query is a tree query.",
      fix: "Save the query as a flat list.",
    });
    expect(watcher()).toHaveTextContent("Work Item Watcher: Error. The saved query is a tree query.");
    expect(watcher()).toHaveTextContent("Save the query as a flat list.");

    await s.reportWorkItemWatcher("acme", { state: "watching" });
    expect(watcher()).toHaveTextContent(`Work Item Watcher: Watching ${QUERY}.`);
  });

  it("is read-only on an Archived Project", async () => {
    const { w, s } = await fresh();
    await s.archive("acme");
    await renderApp(w);
    expect(toggle("Azure Boards")).toBeDisabled();
  });
});
