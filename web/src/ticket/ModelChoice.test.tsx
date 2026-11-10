// The Ticket detail's Model and effort (code-central-313): the model and
// effort the Ticket's next turn runs with, each with its source, and
// pickers that write the Ticket's model and effort Tags or clear them.
// Against FakeApi.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { ReportedModels } from "../api/types";
import { TicketDetail } from "./TicketDetail";

const report = { effort_levels: [], aliases: [], model_flag: "", effort_flag: "" };

// seeded is acme-1, a Ready implementation Ticket, and acme-2, a grilling
// one, on a fresh Project.
function seeded() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Fix login", state: "Ready" });
  fake.addTicket("acme", { title: "Grill it", state: "Ready", labels: ["stage:grilling"] });
  return fake;
}

const reported = async (fake: FakeApi): Promise<ReportedModels> => (await fake.listProjects())[0].reported_models;

async function renderDetail(fake: FakeApi, id = "acme-1") {
  const t = await fake.getTicket("acme", id);
  const r = render(<TicketDetail api={fake} project="acme" ticket={t} reported={await reported(fake)} now={fake.now} />);
  return { ...r, rerender: async () => r.rerender(<TicketDetail api={fake} project="acme" ticket={await fake.getTicket("acme", id)} reported={await reported(fake)} now={fake.now} />) };
}

const section = () => screen.getByRole("group", { name: "Model and effort" });
// applies is what a value's line says applies, with its source.
const applies = (what: "Model" | "Effort") => within(section()).getByTestId(`${what.toLowerCase()}-applies`).textContent;
const modelPicker = () => within(section()).getByLabelText("Model Tag");
const effortPicker = () => within(section()).getByRole("combobox", { name: "Effort Tag" });
const options = (select: HTMLElement) => within(select).getAllByRole("option").map((o) => o.textContent);
const suggested = (input: HTMLElement) => {
  const list = input.getAttribute("list");
  return list ? Array.from(document.getElementById(list)?.querySelectorAll("option") ?? [], (o) => o.value) : [];
};

describe("the Ticket detail's Model and effort", () => {
  it("shows Claude Code's default when nothing sets them and the Factory reported no flag", async () => {
    await renderDetail(seeded());
    expect(applies("Model")).toBe("Model: Claude Code's default");
    expect(applies("Effort")).toBe("Effort: Claude Code's default");
  });

  it("shows the Factory's flag when nothing in Code Central sets them", async () => {
    const fake = seeded();
    await fake.reportModels("acme", { ...report, model_flag: "sonnet", effort_flag: "medium" });
    await renderDetail(fake);
    expect(applies("Model")).toBe("Model: sonnet (Factory flag)");
    expect(applies("Effort")).toBe("Effort: medium (Factory flag)");
  });

  it("names the Project's row for the Ticket's kind of work, or the Project default", async () => {
    const fake = seeded();
    await fake.reportModels("acme", { ...report, model_flag: "sonnet" });
    await fake.setModels("acme", { default: { model: "", effort: "low" }, kinds: [{ kind: "grilling", model: "opus", effort: "" }] });
    await renderDetail(fake, "acme-2");
    expect(applies("Model")).toBe("Model: opus (the Project's grilling row)");
    expect(applies("Effort")).toBe("Effort: low (Project default)");
  });

  it("shows the Ticket's Tags first", async () => {
    const fake = seeded();
    await fake.setModels("acme", { default: { model: "sonnet", effort: "low" }, kinds: [] });
    await fake.setTags("acme", "acme-1", { model: "claude-opus-5-5", effort: "max" });
    await renderDetail(fake);
    expect(applies("Model")).toBe("Model: claude-opus-5-5 (Ticket Tag)");
    expect(applies("Effort")).toBe("Effort: max (Ticket Tag)");
    expect(modelPicker()).toHaveValue("claude-opus-5-5");
    expect(effortPicker()).toHaveValue("max");
  });

  it("offers the Reported Models: aliases and model IDs to the model, the valid levels to the effort", async () => {
    const fake = seeded();
    await fake.reportModels("acme", { ...report, effort_levels: ["low", "high", "turbo"], aliases: ["opus", "sonnet"] });
    fake.seedModelIDs("acme", ["claude-opus-5-5", "opus"]);
    await renderDetail(fake);
    expect(suggested(modelPicker())).toEqual(["opus", "sonnet", "claude-opus-5-5"]);
    expect(modelPicker()).toHaveValue("");
    expect(options(effortPicker())).toEqual(["Project's settings", "low", "high", "turbo"]);
    expect(effortPicker()).toHaveValue("");
  });

  it("offers only free text for the model, and the seeded levels, before any Factory has reported", async () => {
    await renderDetail(seeded());
    expect(modelPicker().getAttribute("list")).toBeNull();
    expect(options(effortPicker())).toEqual(["Project's settings", "low", "medium", "high", "xhigh", "max"]);
  });

  it("writes the model Tag, free text too, and clears it so the Ticket goes back to its Project's settings", async () => {
    const fake = seeded();
    await fake.setModels("acme", { default: { model: "sonnet", effort: "" }, kinds: [] });
    const r = await renderDetail(fake);
    expect(within(section()).queryByRole("button", { name: "Clear model" })).toBeNull();

    // "[[" types a "[".
    await userEvent.type(modelPicker(), " some-future-model[[1m] ");
    await userEvent.click(within(section()).getByRole("button", { name: "Set model" }));
    expect((await fake.getTicket("acme", "acme-1")).tags).toEqual({ model: "some-future-model[1m]" });
    expect(fake.calls).toContain("setTags");

    await r.rerender();
    expect(applies("Model")).toBe("Model: some-future-model[1m] (Ticket Tag)");
    await userEvent.click(within(section()).getByRole("button", { name: "Clear model" }));
    expect((await fake.getTicket("acme", "acme-1")).tags).toEqual({});
    expect(fake.calls).toContain("removeTags");
    await r.rerender();
    expect(applies("Model")).toBe("Model: sonnet (Project default)");
    expect(modelPicker()).toHaveValue("");
  });

  it("writes the effort Tag on picking a level, and clears it on picking the Project's settings", async () => {
    const fake = seeded();
    const r = await renderDetail(fake);
    await userEvent.selectOptions(effortPicker(), "xhigh");
    expect((await fake.getTicket("acme", "acme-1")).tags).toEqual({ effort: "xhigh" });

    await r.rerender();
    await userEvent.selectOptions(effortPicker(), "Project's settings");
    expect((await fake.getTicket("acme", "acme-1")).tags).toEqual({});
  });

  it("shows a rejected effort's error", async () => {
    const fake = seeded();
    await renderDetail(fake);
    // The Factory reports other levels after the panel was read.
    await fake.reportModels("acme", { ...report, effort_levels: ["low", "high"] });
    await userEvent.selectOptions(effortPicker(), "max");
    expect(await within(section()).findByRole("alert")).toHaveTextContent('effort "max" isn\'t a valid level; use one of low, high');
    expect((await fake.getTicket("acme", "acme-1")).tags).toEqual({});
  });
});

describe("the Ticket panel's Model and effort, live", () => {
  it("follows the Ticket's Tags, the Models rows and the Reported Models", async () => {
    const fake = seeded();
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    render(<App api={fake} now={() => fake.now} />);
    const line = async (text: string) => expect(await screen.findByText(text)).toBeInTheDocument();
    await line("Model: Claude Code's default");

    await fake.reportModels("acme", { ...report, model_flag: "sonnet" });
    await line("Model: sonnet (Factory flag)");
    await fake.setModels("acme", { default: { model: "", effort: "" }, kinds: [{ kind: "implementation", model: "opus", effort: "" }] });
    await line("Model: opus (the Project's implementation row)");
    await fake.setTags("acme", "acme-1", { model: "fable" });
    await line("Model: fable (Ticket Tag)");
  });
});
