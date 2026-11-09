// Options view tests: the real App against FakeApi, at ?view=options. They
// check what the human sees and does: opening it from the Inbox's Admin
// section, saving the Focus Mode Option, a refused or failed
// save, returning to where they came from, and a change made elsewhere.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { ApiError } from "../api/types";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Pick a colour", state: "Ready" }); // acme-1
  return fake;
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const options = () => screen.getByRole("main", { name: "Options" });
const focusControl = () => within(options()).getByRole("radiogroup", { name: "Focus Mode, when nothing needs you" });
const choice = (name: string) => within(focusControl()).getByRole("radio", { name });

describe("opening and leaving the Options view", () => {
  it("opens from the Inbox's Admin section and returns to the Inbox with its filter and Ticket", async () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Ready&ticket=acme-1");
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("link", { name: "Options" }));

    expect(options()).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Filters" })).toBeNull();
    expect(await within(focusControl()).findByRole("radio", { name: "All clear", checked: true })).toBeInTheDocument();

    await userEvent.click(within(options()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Ready&ticket=acme-1");
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to Focus Mode with its Project when the address names it", async () => {
    renderApp(world(), "/?view=options&from=focus&project=acme");
    await userEvent.click(within(options()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(screen.getByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to the Inbox when opened straight from the URL", async () => {
    renderApp(world(), "/?view=options");
    await userEvent.click(within(options()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox");
  });

  it("returns to where the human was on Back", async () => {
    renderApp(world(), "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Options" }));
    expect(options()).toBeInTheDocument();

    act(() => window.history.back());
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });
});

describe("the Focus Mode Option", () => {
  it("offers All clear and Activity Dashboard, and saves a choice at once", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());
    expect(choice("Activity Dashboard")).not.toBeChecked();

    await userEvent.click(choice("Activity Dashboard"));
    await waitFor(() => expect(choice("Activity Dashboard")).toBeChecked());
    expect(await fake.options()).toEqual({ focus_clear: "activity_dashboard" });
    expect(within(options()).queryByRole("alert")).toBeNull();

    await userEvent.click(choice("All clear"));
    await waitFor(async () => expect(await fake.options()).toEqual({ focus_clear: "all_clear" }));
    expect(choice("All clear")).toBeChecked();
  });

  it("shows the error when a save is refused, and keeps showing the saved value", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());
    vi.spyOn(fake, "setOptions").mockRejectedValue(new ApiError(422, "invalid", "focus_clear is not an Option's value", "focus_clear"));

    await userEvent.click(choice("Activity Dashboard"));
    expect(await within(options()).findByRole("alert")).toHaveTextContent("focus_clear is not an Option's value");
    expect(choice("All clear")).toBeChecked();
    expect(await fake.options()).toEqual({ focus_clear: "all_clear" });
  });

  it("shows the error when the service can't be reached", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());
    vi.spyOn(fake, "setOptions").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));

    await userEvent.click(choice("Activity Dashboard"));
    expect(await within(options()).findByRole("alert")).toHaveTextContent("Code Central isn't reachable");
  });

  it("clears the error once a later save succeeds", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());
    vi.spyOn(fake, "setOptions").mockRejectedValueOnce(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));

    await userEvent.click(choice("Activity Dashboard"));
    await within(options()).findByRole("alert");
    await userEvent.click(choice("Activity Dashboard"));
    await waitFor(() => expect(choice("Activity Dashboard")).toBeChecked());
    expect(within(options()).queryByRole("alert")).toBeNull();
  });

  it("shows a value changed elsewhere once options.changed arrives", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());

    // Another browser's change: setOptions sends options.changed.
    await act(() => fake.setOptions({ focus_clear: "activity_dashboard" }));
    await waitFor(() => expect(choice("Activity Dashboard")).toBeChecked());
  });

  it("refetches the Options on a reconnect, apart from the Inbox data", async () => {
    const fake = world();
    renderApp(fake, "/?view=options");
    await waitFor(() => expect(choice("All clear")).toBeChecked());
    const before = fake.calls.filter((c) => c === "options").length;

    act(() => fake.reconnect());
    await waitFor(() => expect(fake.calls.filter((c) => c === "options").length).toBe(before + 1));
  });

  it("shows the error when the Options can't be read, with no choice made", async () => {
    const fake = world();
    vi.spyOn(fake, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    renderApp(fake, "/?view=options");
    expect(await within(options()).findByRole("alert")).toHaveTextContent("Code Central isn't reachable");
    expect(choice("All clear")).not.toBeChecked();
    expect(choice("Activity Dashboard")).not.toBeChecked();
  });
});
