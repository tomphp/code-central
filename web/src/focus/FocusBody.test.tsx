// Focus Mode's body's tests, through its props: its header, "N left", the
// Focus Item on screen, All clear, only skipped and loading, in one Project
// and in All Projects, with the data read from a WASM Core world.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { UiLocation } from "../url";
import { FocusBody, type FocusBodyProps, type FocusBodySession } from "./FocusBody";
import { focusData, focusSession, type FocusData } from "./storyData";

let w: World;
let f: FocusData;
beforeEach(async () => {
  w = await newWorld();
  f = await focusData(scenario(w), w);
});
afterEach(() => w.drop());

function renderBody(more: Partial<FocusBodyProps> = {}, over: Partial<FocusBodySession> = {}) {
  const navigate = vi.fn<(patch: Partial<UiLocation>) => void>();
  const s = focusSession(f, { skip: vi.fn(), again: vi.fn(), say: vi.fn(), ...over });
  render(<FocusBody api={f.api} now={f.now} data={f.data} session={s} loc={{ view: "focus" }} navigate={navigate} {...more} />);
  return { navigate, session: s };
}

const focus = () => screen.getByRole("main", { name: "Focus Mode" });
const header = () => within(focus()).getByRole("banner");

describe("Focus Mode's body with a queue", () => {
  it("shows the Focus Item on screen and how many are left", () => {
    renderBody();
    expect(within(header()).getByText("2 left")).toBeInTheDocument();
    const item = within(focus()).getByRole("article", { name: "Focus Item" });
    expect(within(item).getByRole("heading", { level: 2, name: "Flaky build" })).toBeInTheDocument();
    expect(item).toHaveTextContent("Focus: Failed");
  });

  it("sends the item to the back with Skip", async () => {
    const { session } = renderBody();
    await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
    expect(session.skip).toHaveBeenCalledOnce();
  });

  it("names no Project in its header in All Projects", () => {
    renderBody();
    expect(header()).not.toHaveTextContent("Acme");
  });

  it("names its Project in its header in one Project", () => {
    renderBody({ loc: { view: "focus", project: "acme" } });
    expect(header()).toHaveTextContent("Acme acme");
  });

  it("shows the session's note", () => {
    renderBody({}, { note: "Requirement added" });
    expect(within(focus()).getByRole("status")).toHaveTextContent("Requirement added");
  });

  it("has the session say what + New ▾ added", async () => {
    const { session } = renderBody({ loc: { view: "focus", project: "acme" } });
    await userEvent.click(within(header()).getByRole("button", { name: "New requirement" }));
    const modal = screen.getByRole("dialog", { name: "New requirement" });
    await userEvent.type(within(modal).getByRole("textbox"), "Offline mode");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit requirement" }));
    await waitFor(() => expect(session.say).toHaveBeenCalledWith("Requirement added"));
  });

  it("opens the item's Ticket in the Inbox", async () => {
    const { navigate } = renderBody();
    await userEvent.click(within(focus()).getByRole("link", { name: "Open in Inbox" }));
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", project: undefined, requirement: undefined, ticket: f.item.row.id });
  });

  it("goes back to the Inbox on Exit", async () => {
    const { navigate } = renderBody();
    await userEvent.click(within(header()).getByRole("button", { name: "Exit" }));
    expect(navigate).toHaveBeenCalledWith({ view: "inbox" });
  });
});

describe("Focus Mode's body with nothing to show", () => {
  it("says All clear, with no count, when nothing needs the human", () => {
    renderBody({}, { item: undefined, ticket: undefined, remaining: 0, clear: true });
    const clear = within(focus()).getByRole("region", { name: "All clear" });
    expect(within(clear).getByRole("heading", { level: 2, name: "All clear" })).toBeInTheDocument();
    expect(clear).toHaveTextContent("Nothing needs you.");
    expect(within(header()).queryByText(/left$/)).toBeNull();
  });

  it("shows the dashboard it is given in place of All clear", () => {
    renderBody({ dashboard: <section aria-label="Activity Dashboard" /> }, { item: undefined, remaining: 0, clear: true });
    expect(within(focus()).getByRole("region", { name: "Activity Dashboard" })).toBeInTheDocument();
    expect(within(focus()).queryByRole("region", { name: "All clear" })).toBeNull();
  });

  it("offers to go through the skipped items again when only they are left", async () => {
    const { session } = renderBody({}, { item: undefined, onlySkipped: true });
    const skipped = within(focus()).getByRole("region", { name: "Only skipped Focus Items left" });
    expect(skipped).toHaveTextContent("You skipped 2, go through them again?");
    await userEvent.click(within(skipped).getByRole("button", { name: "Go through them again" }));
    expect(session.again).toHaveBeenCalledOnce();
  });
});

describe("Focus Mode's body loading", () => {
  it("shows Loading… and no count before the data", () => {
    renderBody({ data: undefined }, { item: undefined, remaining: 0 });
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(within(header()).queryByText(/left$/)).toBeNull();
  });

  it("shows Loading… while the next item is on its way", () => {
    renderBody({}, { item: undefined });
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(within(header()).getByText("2 left")).toBeInTheDocument();
  });
});
