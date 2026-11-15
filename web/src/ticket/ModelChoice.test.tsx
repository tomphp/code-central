// The Ticket detail's Model and effort (Spec code-central-301): the model and
// effort the Ticket's next turn runs with, each with its source, and
// pickers that write the Ticket's model and effort Tags or clear them.
// On a WASM Core world.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { ReportedModels } from "../api/types";
import { options, suggested } from "../testPickers";
import { renderLive } from "../worldTesting";
import { TicketDetail } from "./TicketDetail";

const report = { effort_levels: [], aliases: [], model_flag: "", effort_flag: "" };

// seeded is a Ready implementation Ticket, fix, and a grilling one, grill,
// on a fresh Project.
async function seeded() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const fix = await s.ticket("acme", { title: "Fix login", state: "Ready" });
  const grill = await s.ticket("acme", { title: "Grill it", state: "Ready", labels: ["stage:grilling"] });
  return { w, s, fix, grill };
}

const reported = async (w: World): Promise<ReportedModels> => (await w.api.listProjects())[0].reported_models;

async function renderDetail(w: World, id: string) {
  const t = await w.api.getTicket("acme", id);
  const r = renderLive(w, <TicketDetail project="acme" ticket={t} reported={await reported(w)} now={w.now()} />);
  return {
    ...r,
    rerender: async () => r.rerender(<TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} reported={await reported(w)} now={w.now()} />),
  };
}

const section = () => screen.getByRole("group", { name: "Model and effort" });
// applies is what a value's note says is used, with its source.
const applies = (what: "Model" | "Effort") => within(section()).getByTestId(`${what.toLowerCase()}-applies`).textContent;
const modelPicker = () => within(section()).getByLabelText("Model Tag");
const effortPicker = () => within(section()).getByRole("combobox", { name: "Effort Tag" });

describe("the Ticket detail's Model and effort", () => {
  it("shows Claude Code's default when nothing sets them and the Factory reported no flag", async () => {
    const { w, fix } = await seeded();
    await renderDetail(w, fix);
    expect(applies("Model")).toBe("Claude Code's default");
    expect(applies("Effort")).toBe("Claude Code's default");
  });

  it("shows the Factory's flag when nothing in Code Central sets them", async () => {
    const { w, s, fix } = await seeded();
    await s.reportModels("acme", { ...report, model_flag: "sonnet", effort_flag: "medium" });
    await renderDetail(w, fix);
    expect(applies("Model")).toBe("sonnet (Factory flag)");
    expect(applies("Effort")).toBe("medium (Factory flag)");
  });

  it("names the Project's row for the Ticket's kind of work, or the Project default", async () => {
    const { w, s, grill } = await seeded();
    await s.reportModels("acme", { ...report, model_flag: "sonnet" });
    await w.api.setModels("acme", { default: { model: "", effort: "low" }, kinds: [{ kind: "grilling", model: "opus", effort: "" }] });
    await renderDetail(w, grill);
    expect(applies("Model")).toBe("opus (the Project's grilling row)");
    expect(applies("Effort")).toBe("low (Project default)");
  });

  it("shows the Ticket's Tags first", async () => {
    const { w, fix } = await seeded();
    await w.api.setModels("acme", { default: { model: "sonnet", effort: "low" }, kinds: [] });
    await w.api.setTags("acme", fix, { model: "claude-opus-5-5", effort: "max" });
    await renderDetail(w, fix);
    expect(applies("Model")).toBe("claude-opus-5-5 (Ticket Tag)");
    expect(applies("Effort")).toBe("max (Ticket Tag)");
    expect(modelPicker()).toHaveValue("claude-opus-5-5");
    expect(effortPicker()).toHaveValue("max");
  });

  it("offers the Reported Models: aliases and model IDs to the model, the valid levels to the effort", async () => {
    const { w, s, fix } = await seeded();
    await s.reportModels("acme", { ...report, effort_levels: ["low", "high", "turbo"], aliases: ["opus", "sonnet"] });
    // A model ID is one a session's Usage has named.
    const at = w.now().toISOString();
    const usage = (model: string) => ({ time: at, kind: "implementation", model, input_tokens: 1, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 1 });
    const used = await s.ticket("acme", { title: "Used", state: "Ready" });
    await s.factorySession("acme", used, { session: "s-used", usage: [usage("claude-opus-5-5"), usage("opus")] });
    await renderDetail(w, fix);
    expect(suggested(modelPicker())).toEqual(["opus", "sonnet", "claude-opus-5-5"]);
    expect(modelPicker()).toHaveValue("");
    expect(options(effortPicker())).toEqual(["Project's settings", "low", "high", "turbo"]);
    expect(effortPicker()).toHaveValue("");
  });

  it("offers only free text for the model, and the seeded levels, before any Factory has reported", async () => {
    const { w, fix } = await seeded();
    await renderDetail(w, fix);
    expect(modelPicker().getAttribute("list")).toBeNull();
    expect(options(effortPicker())).toEqual(["Project's settings", "low", "medium", "high", "xhigh", "max"]);
  });

  it("writes the model Tag, free text too, and clears it so the Ticket goes back to its Project's settings", async () => {
    const { w, fix } = await seeded();
    await w.api.setModels("acme", { default: { model: "sonnet", effort: "" }, kinds: [] });
    const r = await renderDetail(w, fix);
    expect(within(section()).queryByRole("button", { name: "Clear model" })).toBeNull();

    // "[[" types a "[".
    await userEvent.type(modelPicker(), " some-future-model[[1m] ");
    await userEvent.click(within(section()).getByRole("button", { name: "Set model" }));
    expect((await w.api.getTicket("acme", fix)).tags).toEqual({ model: "some-future-model[1m]" });

    await r.rerender();
    expect(applies("Model")).toBe("some-future-model[1m] (Ticket Tag)");
    await userEvent.click(within(section()).getByRole("button", { name: "Clear model" }));
    expect((await w.api.getTicket("acme", fix)).tags).toEqual({});
    await r.rerender();
    expect(applies("Model")).toBe("sonnet (Project default)");
    expect(modelPicker()).toHaveValue("");
  });

  it("writes the effort Tag on picking a level, and clears it on picking the Project's settings", async () => {
    const { w, fix } = await seeded();
    const r = await renderDetail(w, fix);
    await userEvent.selectOptions(effortPicker(), "xhigh");
    expect((await w.api.getTicket("acme", fix)).tags).toEqual({ effort: "xhigh" });

    await r.rerender();
    await userEvent.selectOptions(effortPicker(), "Project's settings");
    expect((await w.api.getTicket("acme", fix)).tags).toEqual({});
  });

  it("shows a rejected effort's error", async () => {
    const { w, s, fix } = await seeded();
    await renderDetail(w, fix);
    // The Factory reports other levels after the panel was read.
    await s.reportModels("acme", { ...report, effort_levels: ["low", "high"] });
    await userEvent.selectOptions(effortPicker(), "max");
    expect(await within(section()).findByRole("alert")).toHaveTextContent('effort "max" isn\'t a valid level; use one of low, high');
    expect((await w.api.getTicket("acme", fix)).tags).toEqual({});
  });
});

describe("the Ticket panel's Model and effort, live", () => {
  it("follows the Ticket's Tags, the Models rows and the Reported Models", async () => {
    const { w, s, fix } = await seeded();
    window.history.replaceState(null, "", `/?view=inbox&ticket=${fix}`);
    render(<App api={w.api} now={w.now} />);
    const line = (text: string) => waitFor(() => expect(applies("Model")).toBe(text));
    await line("Claude Code's default");

    await s.reportModels("acme", { ...report, model_flag: "sonnet" });
    await line("sonnet (Factory flag)");
    await w.api.setModels("acme", { default: { model: "", effort: "" }, kinds: [{ kind: "implementation", model: "opus", effort: "" }] });
    await line("opus (the Project's implementation row)");
    await w.api.setTags("acme", fix, { model: "fable" });
    await line("fable (Ticket Tag)");
  });
});
