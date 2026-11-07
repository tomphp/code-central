// Renaming a Project's display name from its menu in the left pane
// (story 92). Against FakeApi.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/");
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });

async function rename(name: string) {
  await userEvent.click(await within(nav()).findByRole("button", { name: `Menu for ${name}` }));
  await userEvent.click(within(screen.getByRole("menu", { name: `Menu for ${name}` })).getByRole("menuitem", { name: "Rename" }));
  return within(nav()).getByRole("textbox", { name: `Display name for ${name}` });
}

describe("Renaming a Project", () => {
  it("renames a Project's display name from its menu", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    const box = await rename("Acme");
    expect(box).toHaveValue("Acme");
    await userEvent.clear(box);
    await userEvent.type(box, "Acme Web{Enter}");

    expect(await within(nav()).findByRole("button", { name: /^Acme Web/ })).toBeInTheDocument();
    expect(within(nav()).queryByRole("textbox")).toBeNull();
    expect((await fake.listProjects())[0]).toMatchObject({ key: "acme", display_name: "Acme Web" });
  });

  it("keeps the old name on Cancel, and shows a refused rename's reason", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    await userEvent.type(await rename("Acme"), " Corp");
    await userEvent.click(within(nav()).getByRole("button", { name: "Cancel" }));
    expect(within(nav()).queryByRole("textbox")).toBeNull();
    expect(fake.calls).not.toContain("renameProject");

    const box = await rename("Acme");
    await userEvent.clear(box);
    await userEvent.type(box, " ");
    await userEvent.click(within(nav()).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(within(nav()).getByRole("alert")).toHaveTextContent("a Project needs a display name"));
    expect(within(nav()).getByRole("textbox", { name: "Display name for Acme" })).toBeInTheDocument();
  });

  it("offers no rename for an Archived Project, which takes no writes", async () => {
    const fake = new FakeApi();
    fake.addProject("old", "Old App", { archived: true });
    renderApp(fake);
    await userEvent.click(await within(nav()).findByRole("button", { name: "Menu for Old App", hidden: true }));
    expect(within(screen.getByRole("menu", { hidden: true })).queryByRole("menuitem", { name: "Rename", hidden: true })).toBeNull();
  });
});
