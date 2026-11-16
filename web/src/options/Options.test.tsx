// Options view tests: the real App on the WASM Core, at ?view=options. They
// check what the human sees and does: opening it from the Inbox's Admin
// section, saving the Focus Mode Option and the gh path Option, a refused
// or failed save, returning to where they came from, and a change made
// elsewhere.
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { ApiError } from "../api/types";
import { renderApp, scenario, settle } from "../worldTesting";

// world is a WASM Core with the Project acme and one Ready Ticket, whose
// ID is the one the write returned.
async function world() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const ticket = await s.ticket("acme", { title: "Pick a colour", state: "Ready" });
  return { w, ticket };
}

const options = () => screen.getByRole("main", { name: "Options" });
const focusControl = () => within(options()).getByRole("radiogroup", { name: "Focus Mode, when nothing needs you" });
const choice = (name: string) => within(focusControl()).getByRole("radio", { name });

describe("opening and leaving the Options view", () => {
  it("opens from the Inbox's Admin section and returns to the Inbox with its filter and Ticket", async () => {
    const { w, ticket } = await world();
    await renderApp(w, `/?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("link", { name: "Options" }));
    await settle(w);

    expect(options()).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Filters" })).toBeNull();
    expect(within(focusControl()).getByRole("radio", { name: "Activity Dashboard", checked: true })).toBeInTheDocument();

    await userEvent.click(within(options()).getByRole("button", { name: "Back to the Inbox" }));
    await settle(w);
    expect(window.location.search).toBe(`?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to Focus Mode with its Project when the address names it", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options&from=focus&project=acme");
    await userEvent.click(within(options()).getByRole("button", { name: "Back to Focus Mode" }));
    await settle(w);
    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(screen.getByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to the Inbox when opened straight from the URL", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    await userEvent.click(within(options()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox");
  });

  it("returns to where the human was on Back", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Options" }));
    await settle(w);
    expect(options()).toBeInTheDocument();

    // jsdom traverses history on a later task and fires popstate then.
    const popped = new Promise((r) => window.addEventListener("popstate", r, { once: true }));
    await act(async () => {
      window.history.back();
      await popped;
    });
    await settle(w);
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });
});

describe("the Focus Pop-up Option", () => {
  it("follows the Focus Mode entry, offers Off then On, and saves a choice at once", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    const group = within(options()).getByRole("radiogroup", { name: "Pop up Focus Items on other views" });
    const groups = within(options()).getAllByRole("radiogroup");
    expect(groups.indexOf(group)).toBe(groups.indexOf(focusControl()) + 1);
    expect(within(group).getAllByRole("radio").map((r) => r.getAttribute("value"))).toEqual(["off", "on"]);
    expect(within(group).getByRole("radio", { name: "Off" })).toBeChecked();
    expect(within(group).getByRole("radio", { name: "Off" })).toHaveAccessibleDescription("Needs Me waits for you in Focus Mode or a Board card.");
    expect(within(group).getByRole("radio", { name: "On" })).toHaveAccessibleDescription(
      "A popup shows each new Focus Item on every view but Focus Mode, Options and Project Options.",
    );

    await userEvent.click(within(group).getByRole("radio", { name: "On" }));
    await settle(w);
    expect(within(group).getByRole("radio", { name: "On" })).toBeChecked();
    expect(await w.api.options()).toMatchObject({ focus_popup: "on" });

    // A change made elsewhere is followed live.
    await w.api.setOptions({ focus_popup: "off" });
    await settle(w);
    expect(within(group).getByRole("radio", { name: "Off" })).toBeChecked();
  });
});

describe("the Focus Mode Option", () => {
  it("offers Activity Dashboard first, then All clear, and saves a choice at once", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    expect(choice("Activity Dashboard")).toBeChecked();
    expect(within(focusControl()).getAllByRole("radio").map((r) => r.getAttribute("value"))).toEqual(["activity_dashboard", "all_clear"]);
    expect(choice("All clear")).not.toBeChecked();

    await userEvent.click(choice("All clear"));
    await settle(w);
    expect(choice("All clear")).toBeChecked();
    expect(await w.api.options()).toMatchObject({ focus_clear: "all_clear" });
    expect(within(options()).queryByRole("alert")).toBeNull();

    await userEvent.click(choice("Activity Dashboard"));
    await settle(w);
    expect(await w.api.options()).toMatchObject({ focus_clear: "activity_dashboard" });
    expect(choice("Activity Dashboard")).toBeChecked();
  });

  it("shows the error when a save is refused, and keeps showing the saved value", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    vi.spyOn(w.api, "setOptions").mockRejectedValue(new ApiError(422, "invalid", "focus_clear is not an Option's value", "focus_clear"));

    await userEvent.click(choice("All clear"));
    await settle(w);
    expect(within(options()).getByRole("alert")).toHaveTextContent("focus_clear is not an Option's value");
    expect(choice("Activity Dashboard")).toBeChecked();
    expect(await w.api.options()).toMatchObject({ focus_clear: "activity_dashboard" });
  });

  it("shows the error when the service can't be reached", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    vi.spyOn(w.api, "setOptions").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));

    await userEvent.click(choice("All clear"));
    await settle(w);
    expect(within(options()).getByRole("alert")).toHaveTextContent("Code Central isn't reachable");
  });

  it("clears the error once a later save succeeds", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    vi.spyOn(w.api, "setOptions").mockRejectedValueOnce(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));

    await userEvent.click(choice("All clear"));
    await settle(w);
    expect(within(options()).getByRole("alert")).toBeInTheDocument();
    await userEvent.click(choice("All clear"));
    await settle(w);
    expect(choice("All clear")).toBeChecked();
    expect(within(options()).queryByRole("alert")).toBeNull();
  });

  it("shows a value changed elsewhere once options.changed arrives", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    expect(choice("Activity Dashboard")).toBeChecked();

    // Another browser's change: setOptions sends options.changed.
    await w.api.setOptions({ focus_clear: "all_clear" });
    await settle(w);
    expect(choice("All clear")).toBeChecked();
  });

  it("refetches the Options on a reconnect, apart from the Inbox data", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    const before = w.calls.filter((c) => c === "options").length;

    w.dropStreams();
    await settle(w);
    expect(w.calls.filter((c) => c === "options").length).toBe(before + 1);
  });

  it("shows the error when the Options can't be read, with no choice made", async () => {
    const { w } = await world();
    vi.spyOn(w.api, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    await renderApp(w, "/?view=options");
    expect(within(options()).getByRole("alert")).toHaveTextContent("Code Central isn't reachable");
    expect(choice("Activity Dashboard")).not.toBeChecked();
    expect(choice("All clear")).not.toBeChecked();
  });
});

describe("the gh path Option", () => {
  const ghPath = () => within(options()).getByRole("group", { name: "gh path" });
  const field = () => within(ghPath()).getByRole("textbox", { name: "gh path" });

  it("is empty by default, saves a path and clears it", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    expect(field()).toBeEnabled();
    expect(field()).toHaveValue("");
    expect(field()).toHaveAccessibleDescription(/service's PATH, then in \/opt\/homebrew\/bin/);
    expect(within(ghPath()).getByRole("button", { name: "Save" })).toBeDisabled();

    await userEvent.type(field(), "/opt/tools/gh");
    await userEvent.click(within(ghPath()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(ghPath()).getByRole("status")).toHaveTextContent("Saved");
    expect((await w.api.options()).gh_path).toBe("/opt/tools/gh");

    await userEvent.clear(field());
    await userEvent.click(within(ghPath()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await w.api.options()).gh_path).toBe("");
    expect(field()).toHaveValue("");
  });

  it("shows the service's refusal on the field", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    expect(field()).toBeEnabled();

    await userEvent.type(field(), "bin/gh");
    await userEvent.click(within(ghPath()).getByRole("button", { name: "Save" }));
    await settle(w);
    expect(within(options()).getByRole("alert")).toHaveTextContent('gh path "bin/gh" isn\'t an absolute path');
    expect(field()).toHaveAttribute("aria-invalid", "true");
    expect((await w.api.options()).gh_path).toBe("");
  });

  it("shows a path set elsewhere once options.changed arrives", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=options");
    expect(field()).toBeEnabled();
    await w.api.setOptions({ gh_path: "/usr/bin/gh" });
    await settle(w);
    expect(field()).toHaveValue("/usr/bin/gh");
  });
});
