// The Focus section of Project Options (code-central-556): a "Show in
// Focus" checkbox per Needs Me reason, in group order, all on by default;
// Save sends the whole off list. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { NEEDS_ME_REASONS } from "../api/types";
import { REASON_LABEL } from "../format";
import { renderApp as renderAt, scenario, settle } from "../worldTesting";

const renderApp = (w: World) => renderAt(w, "/?view=project-options&project=acme");

const page = () => screen.getByRole("main", { name: "Project Options" });
const focus = () => within(page()).getByRole("group", { name: "Focus" });
const box = (reason: string) => within(focus()).getByRole("checkbox", { name: reason });
const save = async (w: World) => {
  await userEvent.click(within(focus()).getByRole("button", { name: "Save" }));
  await settle(w);
};
const fresh = async (opts: { archived?: boolean } = {}) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (opts.archived) await s.archive("acme");
  return w;
};

describe("the Focus section", () => {
  it("comes after Stage Guides, with a checked box per reason in group order", async () => {
    await renderApp(await fresh());

    const sections = Array.from(page().querySelectorAll("fieldset.project-options-section > legend"), (l) => l.textContent);
    expect(sections.slice(sections.indexOf("Stage Guides"))).toEqual(["Stage Guides", "Auto-accept", "Focus", "Factories"]);
    expect(within(focus()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(NEEDS_ME_REASONS.map((r) => REASON_LABEL[r]));
    for (const r of NEEDS_ME_REASONS) expect(box(REASON_LABEL[r])).toBeChecked();
    expect(within(focus()).getByRole("columnheader", { name: "Show in Focus" })).toBeInTheDocument();
    expect(within(focus()).getByRole("button", { name: "Save" })).toBeDisabled();
    expect(focus()).toHaveTextContent(/stays? in Needs Me|stay in Needs Me/);
  });

  it("saves the whole off list, and turns a reason back on", async () => {
    const w = await fresh();
    await renderApp(w);

    await userEvent.click(box("In Review"));
    await userEvent.click(box("Answered"));
    await save(w);
    expect(within(focus()).getByRole("status")).toBeInTheDocument();
    expect((await w.api.listProjects())[0].focus_kinds.off).toEqual(["answered", "in_review"]);
    expect(within(focus()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.click(box("Answered"));
    await save(w);
    expect(within(focus()).getByRole("status")).toBeInTheDocument();
    expect((await w.api.listProjects())[0].focus_kinds.off).toEqual(["in_review"]);
  });

  it("follows the Project as read", async () => {
    const w = await fresh();
    await renderApp(w);

    await w.api.setFocusKinds("acme", ["failed"]);
    await settle(w);
    expect(within(focus()).getByRole("checkbox", { name: "Failed", checked: false })).toBeInTheDocument();
    expect(box("Answered")).toBeChecked();
  });

  it("is disabled while the Project is archived", async () => {
    await renderApp(await fresh({ archived: true }));

    expect(focus()).toBeDisabled();
  });
});
