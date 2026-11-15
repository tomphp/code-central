// SpecPane and RequirementPane take a read Spec as props: these tests render
// them on a WASM Core world's Specs, without the App.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import { RequirementPane } from "../requirement/RequirementPane";
import { renderLive } from "../worldTesting";
import { SpecPane } from "./SpecPane";

const link = () => ({ href: "#", onClick: () => {} });

describe("SpecPane", () => {
  it("says there are no Tickets for a Spec without any", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const spec = await w.api.getSpec("acme", await s.spec("acme", { title: "Sign-in" }));
    renderLive(w, <SpecPane spec={spec} now={w.now()} select={() => {}} />);
    expect(screen.getByRole("heading", { name: "Sign-in", level: 2 })).toBeInTheDocument();
    expect(screen.getByText("No Tickets yet.")).toBeInTheDocument();
  });

  it("lists the Tickets with their States and selects one on click", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const id = await s.spec("acme", { title: "Sign-in" });
    await s.ticket("acme", { title: "Add a page", spec: id, state: "Accepted" });
    const select = vi.fn();
    renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={select} />);
    const list = screen.getByRole("list", { name: "Tickets" });
    expect(list).toHaveTextContent("Accepted");
    await userEvent.click(within(list).getByRole("button", { name: /Add a page/ }));
    expect(select).toHaveBeenCalledWith(expect.stringMatching(/^acme-/));
  });
});

describe("RequirementPane", () => {
  it("shows the Stage strip and no Build list before any implementation Ticket", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec } = await w.api.submitRequirement("acme", "Export a Spec");
    const read = await w.api.getSpec("acme", spec.id);
    renderLive(w, <RequirementPane spec={{ ...read, requirement: read.requirement! }} now={w.now()} select={() => {}} specLink={link} />);
    expect(within(screen.getByRole("list", { name: "Stages" })).getAllByRole("listitem")).toHaveLength(4);
    expect(screen.queryByRole("list", { name: "Build" })).toBeNull();
  });
});
