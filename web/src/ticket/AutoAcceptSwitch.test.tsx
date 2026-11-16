// The Auto-accept switch (Spec code-central-589), an Off/On toggle group, on
// a Ticket's detail and on the Spec and Requirement panes: the real App on a WASM Core world.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import type { State } from "../api/types";
import { renderApp, scenario, settle } from "../worldTesting";

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
// switchIn finds the Auto-accept toggle group, isOn reads it and turn
// presses its On or Off.
const switchIn = (el: HTMLElement) => within(el).getByRole("group", { name: "Auto-accept" });
const isOn = (sw: HTMLElement) => within(sw).getByRole("button", { name: "On" }).getAttribute("aria-pressed") === "true";
const turn = async (w: World, sw: HTMLElement, to: "On" | "Off") => {
  await userEvent.click(within(sw).getByRole("button", { name: to }));
  await settle(w);
};
const buttonsIn = (sw: HTMLElement) => within(sw).getAllByRole("button");

// seeded makes the Project acme with one Spec, "Accounts", labelled as asked.
async function seeded(opts: { specLabels?: string[]; specTitle?: string } = {}) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const spec = await s.spec("acme", { title: opts.specTitle ?? "Accounts", labels: opts.specLabels });
  // Core keeps "auto-accept" for the human, so it is added once the Ticket is made.
  const ticket = async (title: string, state: State, labels: string[] = []) => {
    const id = await s.ticket("acme", { title, state, spec, labels: labels.filter((l) => l !== "auto-accept") });
    if (labels.includes("auto-accept")) await w.api.addLabels("acme", id, ["auto-accept"]);
    return id;
  };
  return { w, s, spec, ticket };
}

const ticketUrl = (id: string) => `/?view=inbox&ticket=${id}`;

describe("the Auto-accept switch on a Ticket", () => {
  it("reads on exactly when the Ticket carries the Label, and turns off at once without asking", async () => {
    const { w, ticket } = await seeded();
    const id = await ticket("Login page", "Ready", ["auto-accept"]);
    await renderApp(w, ticketUrl(id));

    const sw = switchIn(detail());
    expect(isOn(sw)).toBe(true);
    await turn(w, sw, "Off");
    expect(isOn(sw)).toBe(false);
    expect((await w.api.getTicket("acme", id)).labels).toEqual([]);
  });

  it("turns on at once when it would accept nothing now In Review", async () => {
    const { w, ticket } = await seeded();
    const id = await ticket("Login page", "Ready");
    await renderApp(w, ticketUrl(id));

    const sw = switchIn(detail());
    expect(isOn(sw)).toBe(false);
    await turn(w, sw, "On");
    expect(isOn(sw)).toBe(true);
    expect(screen.queryByRole("form", { name: "Turn on auto-accept" })).toBeNull();
    expect((await w.api.getTicket("acme", id)).labels).toEqual(["auto-accept"]);
  });

  it("asks first for a Ticket In Review, accepts it on Turn on, and writes nothing on Cancel", async () => {
    const { w, ticket } = await seeded();
    const id = await ticket("Login page", "In Review");
    await renderApp(w, ticketUrl(id));

    const sw = switchIn(detail());
    await turn(w, sw, "On");
    const box = screen.getByRole("form", { name: "Turn on auto-accept" });
    expect(box).toHaveTextContent("This will accept 1 Ticket now In Review. Turn on?");
    await userEvent.click(within(box).getByRole("button", { name: "Cancel" }));
    await settle(w);
    expect(screen.queryByRole("form", { name: "Turn on auto-accept" })).toBeNull();
    expect(isOn(sw)).toBe(false);
    expect((await w.api.getTicket("acme", id)).labels).toEqual([]);

    await turn(w, sw, "On");
    await userEvent.click(within(screen.getByRole("form", { name: "Turn on auto-accept" })).getByRole("button", { name: "Turn on" }));
    await settle(w);
    expect((await w.api.getTicket("acme", id)).state).toBe("Accepted");
    expect((await w.api.getTicket("acme", id)).moves!.at(-1)).toMatchObject({ actor: "human", cause: "ticket-label" });
  });

  it("notes that the Spec's Label covers the Ticket, linking the Spec, while the switch reads off", async () => {
    const { w, spec, ticket } = await seeded({ specLabels: ["auto-accept"] });
    const id = await ticket("Login page", "Ready");
    await renderApp(w, ticketUrl(id));

    const sw = switchIn(detail());
    expect(isOn(sw)).toBe(false);
    expect(detail()).toHaveTextContent("Auto-accepted via its Spec's Label");
    // The note's link is the second Spec link after the one in the meta line.
    const links = within(detail()).getAllByRole("button", { name: `${spec} Accounts` });
    expect(links).toHaveLength(2);
  });

  it.each<[string, State, string[], string[]]>([
    ["a Prototype", "Ready", ["prototype"], []],
    ["a Map's Ticket", "Ready", [], ["wayfinder:map"]],
    ["an Accepted Ticket", "Accepted", [], []],
    ["a Done Ticket", "Done", [], []],
    ["a Cancelled Ticket", "Cancelled", [], []],
  ])("is absent on %s", async (_, state, labels, specLabels) => {
    const { w, ticket } = await seeded({ specLabels });
    const id = await ticket("The one", state, labels);
    await renderApp(w, ticketUrl(id));

    expect(within(detail()).getByRole("heading", { name: "The one" })).toBeInTheDocument();
    expect(within(detail()).queryByRole("group", { name: "Auto-accept" })).toBeNull();
  });

  it("shows a refused write's error by the switch and leaves it as it was", async () => {
    const { w, ticket } = await seeded();
    const id = await ticket("Login page", "Ready");
    // Core refuses a Label it doesn't know to the human; here the write fails outright.
    const addLabels = w.api.addLabels;
    w.api.addLabels = async () => {
      throw new Error("the human only");
    };
    await renderApp(w, ticketUrl(id));

    const sw = switchIn(detail());
    await turn(w, sw, "On");
    expect(within(detail()).getByRole("alert")).toHaveTextContent("the human only");
    expect(isOn(sw)).toBe(false);
    w.api.addLabels = addLabels;
  });
});

describe("the Auto-accept switch on a Spec", () => {
  it("asks with the count of the Spec's Tickets In Review, leaving out a Prototype, then accepts them", async () => {
    const { w, spec, ticket } = await seeded();
    const a = await ticket("Login page", "In Review");
    const b = await ticket("Sign-up page", "In Review");
    await ticket("Prototype: picker", "In Review", ["prototype"]);
    await ticket("Logout", "Ready");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = screen.getByRole("region", { name: "Spec" });
    await turn(w, switchIn(pane), "On");
    const box = within(pane).getByRole("form", { name: "Turn on auto-accept" });
    expect(box).toHaveTextContent("This will accept 2 Tickets now In Review. Turn on?");
    await userEvent.click(within(box).getByRole("button", { name: "Turn on" }));
    await settle(w);

    expect((await w.api.getTicket("acme", a)).state).toBe("Accepted");
    expect((await w.api.getTicket("acme", b)).state).toBe("Accepted");
    expect((await w.api.getTicket("acme", a)).moves!.at(-1)).toMatchObject({ cause: "spec-label" });
    expect(isOn(switchIn(pane))).toBe(true);
  });

  it("turns on at once with nothing In Review, and off without asking, on a Long-lived Spec too", async () => {
    const { w, spec, ticket } = await seeded({ specTitle: "Small Changes", specLabels: ["long-lived"] });
    await ticket("Tweak", "Ready");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = screen.getByRole("region", { name: "Spec" });
    const sw = switchIn(pane);
    await turn(w, sw, "On");
    expect(isOn(sw)).toBe(true);
    expect((await w.api.getSpec("acme", spec)).labels).toEqual(["long-lived", "auto-accept"]);
    await turn(w, sw, "Off");
    expect(isOn(sw)).toBe(false);
    expect((await w.api.getSpec("acme", spec)).labels).toEqual(["long-lived"]);
  });

  it("is on a Requirement's pane", async () => {
    const w = await newWorld();
    await scenario(w).project("acme", "Acme");
    const { spec: { id } } = await w.api.submitRequirement("acme", "Export a Spec as Markdown");
    await renderApp(w, `/?view=inbox&requirement=${id}`);

    const sw = switchIn(screen.getByRole("region", { name: "Requirement" }));
    for (const b of buttonsIn(sw)) expect(b).not.toBeDisabled();
    await turn(w, sw, "On");
    expect(isOn(sw)).toBe(true);
    expect((await w.api.getSpec("acme", id)).labels).toContain("auto-accept");
  });

  it("is read-only on an Archived Project", async () => {
    // Core archives a Project only once every Ticket is finished, so the
    // Spec has none.
    const { w, spec } = await seeded();
    await w.api.archiveProject("acme");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    for (const b of buttonsIn(switchIn(screen.getByRole("region", { name: "Spec" })))) expect(b).toBeDisabled();
  });

  it("is absent on a finished Spec", async () => {
    const { w, spec, ticket } = await seeded();
    await ticket("Login page", "Done");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = screen.getByRole("region", { name: "Spec" });
    expect(within(pane).getByText("Login page")).toBeInTheDocument();
    expect(within(pane).queryByRole("group", { name: "Auto-accept" })).toBeNull();
  });

  it("is absent on the Map pane", async () => {
    const { w, spec, ticket } = await seeded({ specLabels: ["wayfinder:map"] });
    await ticket("Chart it", "In Review");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = screen.getByRole("region", { name: "Map" });
    expect(within(pane).queryByRole("group", { name: "Auto-accept" })).toBeNull();
  });
});
