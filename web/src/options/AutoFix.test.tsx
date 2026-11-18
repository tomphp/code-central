// The Auto-fix block in the CI section of Project Options (Spec
// code-central-1022): shown only while CI Tracking is on, saved through
// set_auto_fix, with Go's refusals shown and a way back to the default
// Fix instructions. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import { DEFAULT_FIX_INSTRUCTIONS } from "../api/scenario";
import { renderApp, scenario, settle } from "../worldTesting";

const fresh = async (tracking: boolean) => {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  if (tracking) await s.trackCI("acme");
  await renderApp(w, "/?view=project-options&project=acme");
  return w;
};
const block = () => within(screen.getByRole("group", { name: "CI" })).getByRole("group", { name: "Auto-fix" });
const instructions = () => within(block()).getByRole("textbox", { name: "Fix instructions" });
const limit = () => within(block()).getByRole("spinbutton", { name: "Auto-fix limit" });
const save = () => within(block()).getByRole("button", { name: "Save Auto-fix" });

describe("the Auto-fix block", () => {
  it("is hidden while CI Tracking is off", async () => {
    await fresh(false);
    expect(within(screen.getByRole("group", { name: "CI" })).queryByRole("group", { name: "Auto-fix" })).toBeNull();
  });

  it("starts off, with the default Fix instructions and limit 3", async () => {
    await fresh(true);
    expect(within(block()).getByRole("checkbox", { name: "Auto-fix" })).not.toBeChecked();
    expect(instructions()).toHaveValue(DEFAULT_FIX_INSTRUCTIONS);
    expect(limit()).toHaveValue(3);
    expect(save()).toBeDisabled();
  });

  it("saves the switch, the Fix instructions and the limit", async () => {
    const w = await fresh(true);
    await userEvent.click(within(block()).getByRole("checkbox", { name: "Auto-fix" }));
    await userEvent.clear(instructions());
    await userEvent.type(instructions(), "Fix it.");
    await userEvent.clear(limit());
    await userEvent.type(limit(), "5");
    await userEvent.click(save());
    await settle(w);

    expect(within(block()).getByRole("status")).toHaveTextContent("Saved");
    const af = (await w.api.listProjects()).find((p) => p.key === "acme")!.ci.auto_fix;
    expect([af.on, af.fix_instructions, af.limit]).toEqual([true, "Fix it.", 5]);
  });

  it("shows Go's refusals: a limit out of range, and empty Fix instructions", async () => {
    const w = await fresh(true);
    await userEvent.clear(limit());
    await userEvent.type(limit(), "11");
    await userEvent.click(save());
    await settle(w);
    expect(limit()).toBeInvalid();
    expect(within(block()).getByRole("alert")).toHaveTextContent(/1 to 10/);

    await userEvent.clear(limit());
    await userEvent.type(limit(), "4");
    await userEvent.clear(instructions());
    await userEvent.click(save());
    await settle(w);
    expect(instructions()).toBeInvalid();
    expect(within(block()).getByRole("alert")).toHaveTextContent(/can't be empty/);
  });

  it("goes back to the default Fix instructions", async () => {
    await fresh(true);
    const reset = within(block()).getByRole("button", { name: "Use the default instructions" });
    expect(reset).toBeDisabled();
    await userEvent.clear(instructions());
    await userEvent.type(instructions(), "Mine.");
    await userEvent.click(reset);
    expect(instructions()).toHaveValue(DEFAULT_FIX_INSTRUCTIONS);
  });
});
