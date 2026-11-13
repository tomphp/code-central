// The Focus section of Project Options (code-central-556): a "Show in
// Focus" checkbox per Needs Me reason, in group order, all on by default;
// Save sends the whole off list. Against FakeApi.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { NEEDS_ME_REASONS } from "../api/types";
import { REASON_LABEL } from "../format";

function renderApp(fake: FakeApi) {
  window.history.replaceState(null, "", "/?view=project-options&project=acme");
  return render(<App api={fake} now={() => fake.now} />);
}

const page = () => screen.getByRole("main", { name: "Project Options" });
const focus = () => within(page()).getByRole("group", { name: "Focus" });
const findFocus = async () =>
  within(await screen.findByRole("main", { name: "Project Options" })).findByRole("group", { name: "Focus" });
const box = (reason: string) => within(focus()).getByRole("checkbox", { name: reason });
const save = () => userEvent.click(within(focus()).getByRole("button", { name: "Save" }));
const fresh = (fake: FakeApi) => {
  fake.addProject("acme", "Acme");
  return fake;
};

describe("the Focus section", () => {
  it("comes after Stage Guides, with a checked box per reason in group order", async () => {
    renderApp(fresh(new FakeApi()));

    await findFocus();
    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections.slice(sections.indexOf("Stage Guides"))).toEqual(["Stage Guides", "Focus", "Factories"]);
    expect(within(focus()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(NEEDS_ME_REASONS.map((r) => REASON_LABEL[r]));
    for (const r of NEEDS_ME_REASONS) expect(box(REASON_LABEL[r])).toBeChecked();
    expect(within(focus()).getByRole("columnheader", { name: "Show in Focus" })).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(focus()).toHaveTextContent(/stays? in Needs Me|stay in Needs Me/);
  });

  it("saves the whole off list, and turns a reason back on", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findFocus();
    await userEvent.click(box("In Review"));
    await userEvent.click(box("Answered"));
    await save();
    await within(focus()).findByRole("status");
    expect((await fake.listProjects())[0].focus_kinds.off).toEqual(["answered", "in_review"]);
    expect(within(focus()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.click(box("Answered"));
    await save();
    await within(focus()).findByRole("status");
    expect((await fake.listProjects())[0].focus_kinds.off).toEqual(["in_review"]);
  });

  it("follows the Project as read", async () => {
    const fake = fresh(new FakeApi());
    renderApp(fake);

    await findFocus();
    await fake.setFocusKinds("acme", ["failed"]);
    await within(focus()).findByRole("checkbox", { name: "Failed", checked: false });
    expect(box("Answered")).toBeChecked();
  });

  it("is disabled while the Project is archived", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme", { archived: true });
    renderApp(fake);

    expect(await findFocus()).toBeDisabled();
  });
});
