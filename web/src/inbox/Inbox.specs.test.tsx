// Inbox layout tests for Specs: State lists By Spec, the Spec row and Short IDs.
import { within, act, cleanup } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { type World } from "../api/core/core";
import { scenario, settle } from "../worldTesting";
import { startWorld, done, world, renderApp, list, nav, rowIds, params, requirements, bySpecWorld, specRows, specRow, openIds, specLinkOf, bySpec } from "./Inbox.testutil";

describe("State lists By Spec", () => {
  beforeEach(() => localStorage.clear());

  it("shows the control on State lists only, with By Ticket the default and today's rows", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    const byTicket = within(list()).getByRole("button", { name: "Ticket" });
    expect(byTicket).toHaveAttribute("aria-pressed", "true");
    expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "false");
    expect(rowIds()).toContain("acme-6");

    await userEvent.click(within(nav()).getByRole("button", { name: /^Needs Me/ }));
    expect(within(list()).queryByRole("button", { name: "Spec" })).toBeNull();
  });

  it("groups into collapsed Spec rows in first-Ticket order, with kind, ID, n of m and chips", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    within(list()).getByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    expect(specRows().map((c) => c.getAttribute("aria-label"))).toEqual([
      "Export a Spec as Markdown", // its Stage acme-3 comes first
      "Archive finished Projects",
      "Wayfinding",
      "Beta plan",
    ]);
    expect(rowIds()).toEqual(["acme-1", "acme-5", "acme-9", "beta-1"]); // collapsed: the Spec rows alone

    const archive = specRow("Archive finished Projects");
    expect(within(archive).getByText("Spec")).toBeInTheDocument();
    expect(archive).toHaveTextContent("acme-5");
    expect(archive).toHaveTextContent("2 of 3");
    expect(within(archive).getByText("Ready 2")).toHaveAttribute("aria-current", "true");
    expect(within(archive).getByText("Failed 1")).not.toHaveAttribute("aria-current");
    expect(within(specRow("Wayfinding")).getByText("Map")).toBeInTheDocument();
    // A Requirement shows its Stage strip in place of chips.
    const req = specRow("Export a Spec as Markdown");
    expect(within(req).getByRole("list", { name: "Stages" })).toBeInTheDocument();
    expect(within(req).queryByText("Done 1")).toBeNull();
    expect(req).toHaveTextContent("3 of 4");
  });

  it("opens a Spec row with its Tickets in flat-list order, Stage Tickets marked, no Stage group", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-3", "acme-4", "acme-11"]);
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toHaveTextContent("spec");
    expect(within(list()).queryByText(/run by a Factory/)).toBeNull();
    expect(params().has("requirement")).toBe(false);
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual([]);
  });

  it("opens the selected Ticket's Spec row on its own, which can be closed", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec&ticket=acme-8");
    expect(openIds("Archive finished Projects")).toEqual(["acme-6", "acme-8"]);
    await userEvent.click(within(list()).getByRole("button", { name: "Archive finished Projects" }));
    expect(openIds("Archive finished Projects")).toEqual([]);
  });

  it("opens the Spec from a click on its row", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec&ticket=acme-8");
    within(list()).getByRole("region", { name: "Wayfinding" });
    await userEvent.click(specLinkOf("Wayfinding"));
    expect(params().get("requirement")).toBe("acme-9");
    expect(params().get("ticket")).toBeNull();
    expect(params().get("group")).toBe("spec");
    expect(specLinkOf("Wayfinding")).toHaveAttribute("aria-current", "true");
  });

  it("marks a Spec row whose Ticket needs the human", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Failed&group=spec");
    const archive = within(list()).getByRole("region", { name: "Archive finished Projects" });
    expect(within(archive).getByLabelText("Needs you")).toBeInTheDocument();
    await userEvent.click(within(archive).getByRole("button", { name: "Archive finished Projects" }));
    expect(within(within(list()).getByRole("link", { name: /acme-7/ })).getByLabelText("Needs you")).toBeInTheDocument();
  });

  it("keeps the grouping across lists and Projects, and from the address", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    within(list()).getByRole("button", { name: "Archive finished Projects" });
    await userEvent.click(within(nav()).getByRole("button", { name: /^Failed/ }));
    expect(params().get("group")).toBe("spec");
    expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Needs Me/ }));
    expect(params().get("group")).toBe("spec");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Acme/ }));
    expect(params().get("group")).toBe("spec");
    await userEvent.click(within(nav()).getByRole("button", { name: /^Ready/ }));
    expect(specRows().map((c) => c.getAttribute("aria-label"))).not.toContain("Beta plan");
  });

  it("uses the last pick when the address has none, and By Ticket clears it", async () => {
    const w = await bySpecWorld();
    await renderApp(w, "/?view=inbox&state=Ready");
    within(list()).getByRole("button", { name: "Spec" });
    await bySpec();
    expect(localStorage.getItem("code-central.inbox.group")).toBe("spec");

    window.history.replaceState(null, "", "/?view=inbox&state=Ready");
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    await settle(w);
    expect(within(list()).getByRole("button", { name: "Spec" })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(within(list()).getByRole("button", { name: "Ticket" }));
    expect(params().has("group")).toBe(false);
    expect(localStorage.getItem("code-central.inbox.group")).toBe("ticket");
    expect(rowIds()).toContain("acme-6");
  });

  it("falls back to By Ticket when storage throws", async () => {
    const get = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied");
    });
    const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied");
    });
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready");
    within(list()).getByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    get.mockRestore();
    set.mockRestore();
  });

  it("says so when a State has no Tickets", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Cancelled&group=spec");
    expect(within(list()).getByText("No Tickets Cancelled.")).toBeInTheDocument();
  });

  it("renders only Spec rows for a long collapsed list, newest first inside a Done Spec row", async () => {
    const { w, s } = await startWorld("acme");
    const specs = [];
    for (const t of ["A", "B", "C"]) specs.push(await s.spec("acme", { title: `Spec ${t}` }));
    for (let i = 0; i < 60; i++) {
      const id = await s.ticket("acme", { title: `Done ${i}`, spec: specs[i % 3] });
      await done(s, "acme", id);
      w.advance(1000);
    }
    await renderApp(w, "/?view=inbox&state=Done&group=spec");
    expect(specRows()).toHaveLength(3);
    expect(rowIds()).toHaveLength(3);
    // The newest Ticket (i=59, Spec C) is first, so its Spec row leads.
    expect(specRows()[0]).toHaveAttribute("aria-label", "Spec C");
    await userEvent.click(within(specRows()[0]).getByRole("button", { name: "Spec C" }));
    // Newest first inside the Spec row: Done 59, 56, 53 ...
    const tickets = within(specRows()[0]).getByRole("group", { name: "Tickets of Spec C" });
    expect(within(tickets).getAllByRole("link").slice(0, 2).map((a) => a.textContent)).toEqual([
      expect.stringContaining("Done 59"),
      expect.stringContaining("Done 56"),
    ]);
    expect(rowIds()).toHaveLength(23);
  });

  it("loads every Spec with one findSpecs call per Project", async () => {
    const w = await bySpecWorld();
    w.calls.length = 0;
    await renderApp(w);
    within(nav()).getByRole("button", { name: /^Ready/ });
    expect(w.calls.filter((c) => c === "findSpecs")).toHaveLength(2);
  });
});

describe("the Spec row", () => {
  beforeEach(() => localStorage.clear());

  // withLongLived adds a Long-lived Spec (acme-12) with a Ready Ticket
  // (acme-13) and one a Factory is on (acme-14).
  async function withLongLived(w: World) {
    const s = scenario(w);
    const spec = await s.spec("acme", { title: "CI Fixes" }); // acme-12
    await w.api.addLabels("acme", spec, ["long-lived"]);
    await s.ticket("acme", { title: "Fix the flaky test", spec }); // acme-13
    await s.ticket("acme", { title: "Fix lint", spec, state: "In Progress" }); // acme-14
    return w;
  }

  it("badges every kind but a Requirement, and shows the strip for a Requirement and State chips for the rest", async () => {
    await renderApp(await withLongLived(await bySpecWorld()), "/?view=inbox&state=Ready&group=spec");
    const req = within(list()).getByRole("region", { name: "Export a Spec as Markdown" });
    expect(within(req).getByRole("list", { name: "Stages" })).toBeInTheDocument();
    expect(within(req).queryByText("Requirement")).toBeNull();
    expect(within(req).queryByText(/^Ready \d+$/)).toBeNull();

    for (const [name, kind] of [["Archive finished Projects", "Spec"], ["Wayfinding", "Map"], ["CI Fixes", "Long-lived"]]) {
      expect(within(specRow(name)).getByText(kind)).toBeInTheDocument();
      expect(within(specRow(name)).queryByRole("list", { name: "Stages" })).toBeNull();
      expect(within(specRow(name)).getByText(/^Ready \d+$/)).toHaveAttribute("aria-current", "true");
    }
    expect(within(specRow("CI Fixes")).getByText("In Progress 1")).not.toHaveAttribute("aria-current");
  });

  it("heads a plain Spec with its first need, else a Factory on it, else its Done count alone", async () => {
    await renderApp(await withLongLived(await bySpecWorld()), "/?view=inbox&state=Ready&group=spec");
    const archive = within(list()).getByRole("region", { name: "Archive finished Projects" });
    expect(archive).toHaveTextContent("Failed · 0 of 3 Done");
    expect(specRow("CI Fixes")).toHaveTextContent("A Factory is on it · 0 of 2 Done");
    expect(specRow("CI Fixes")).toHaveTextContent("1 of 2");
    const beta = specRow("Beta plan");
    expect(beta).toHaveTextContent("0 of 1 Done");
    expect(beta).not.toHaveTextContent("·");
    expect(beta).not.toHaveTextContent("1 of 1");
  });

  it("heads a finished Spec with its Done count", async () => {
    const { w, s } = await startWorld("acme");
    const spec = await s.spec("acme", { title: "Shipped" }); // acme-1
    for (const state of ["Done", "Done", "Cancelled"] as const) await s.ticket("acme", { title: state, state, spec });
    await renderApp(w, "/?view=inbox&state=Done&group=spec");
    const row = within(list()).getByRole("region", { name: "Shipped" });
    expect(row).toHaveTextContent("Finished · 2 of 3 Tickets Done");
    expect(row).toHaveTextContent("2 of 3");
  });

  it("dots a Spec that needs the human, and no other", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&state=Ready&group=spec");
    const archive = within(list()).getByRole("region", { name: "Archive finished Projects" });
    // acme-7 needs the human, though it isn't in the Ready list.
    expect(within(archive).getByLabelText("Needs you")).toBeInTheDocument();
    for (const name of ["Export a Spec as Markdown", "Wayfinding", "Beta plan"]) expect(within(specRow(name)).queryByLabelText("Needs you")).toBeNull();
  });

  it("expands a Requirement in the Requirements view to all its Tickets, Stages first, with State chips, without navigating", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&list=requirements");
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-2", "acme-3", "acme-4", "acme-11"]);
    expect(within(list()).getByRole("link", { name: /acme-2/ })).toHaveTextContent("Done");
    expect(within(list()).getByRole("link", { name: /acme-11/ })).toHaveTextContent("Ready");
    expect(params().has("requirement")).toBe(false);
    await userEvent.click(within(list()).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual([]);
  });

  it("shows a Map in the Requirements view with chips and its decision count", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&list=requirements");
    const map = within(list()).getByRole("region", { name: "Wayfinding" });
    expect(within(map).getByText("Map")).toBeInTheDocument();
    expect(within(map).getByText("Ready 1")).toBeInTheDocument();
    expect(map).toHaveTextContent("0/1 decided"); // Core counts the Charting Ticket
    await userEvent.click(within(map).getByRole("button", { name: "Wayfinding" }));
    expect(openIds("Wayfinding")).toEqual(["acme-10"]);
  });

  it("opens the selected Ticket's row on its own in the Requirements view, and a row click opens the Spec", async () => {
    await renderApp(await bySpecWorld(), "/?view=inbox&list=requirements&ticket=acme-11");
    expect(openIds("Export a Spec as Markdown")).toContain("acme-11");
    await userEvent.click(specLinkOf("Export a Spec as Markdown"));
    expect(params().get("requirement")).toBe("acme-1");
    expect(specLinkOf("Export a Spec as Markdown")).toHaveAttribute("aria-current", "true");
  });

  it("expands a Requirement in Needs Me to its Tickets", async () => {
    await renderApp(await requirements());
    const group = within(list()).getByRole("region", { name: "Requirements" });
    await userEvent.click(within(group).getByRole("button", { name: "Export a Spec as Markdown" }));
    expect(openIds("Export a Spec as Markdown")).toEqual(["acme-2", "acme-3", "acme-4"]);
    expect(within(within(list()).getByRole("link", { name: /acme-3/ })).getByLabelText("Needs you")).toBeInTheDocument();
    expect(params().has("requirement")).toBe(false);
  });
});

describe("Short IDs", () => {
  const idIn = (name: string) => within(list()).getByRole("button", { name });

  it("shows #n with one Project selected and the full ID with All Projects, the full ID on hover", async () => {
    await renderApp(await world(), "/?state=Ready&project=acme");
    const short = idIn("acme-5");
    expect(short).toHaveTextContent("#5");
    expect(short).toHaveAttribute("title", "acme-5");
    cleanup();
    await renderApp(await world(), "/?state=Ready");
    expect(idIn("acme-5")).toHaveTextContent("acme-5");
  });

  it("copies the full ID on click, flashes Copied and leaves the URL alone", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    await renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = idIn("acme-5");
    await user.click(id);
    expect(writeText).toHaveBeenCalledWith("acme-5");
    expect(within(list()).getByText("Copied")).toBeInTheDocument();
    expect(window.location.href).toBe(before);
  });

  it("does nothing when there is no clipboard", async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });
    await renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = idIn("acme-5");
    await user.click(id);
    expect(id).toHaveTextContent("#5");
    expect(window.location.href).toBe(before);
  });

  it("does nothing more when the write fails", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    await renderApp(await world(), "/?state=Ready&project=acme");
    const before = window.location.href;
    const id = idIn("acme-5");
    await user.click(id);
    expect(writeText).toHaveBeenCalled();
    expect(id).toHaveTextContent("#5");
    expect(window.location.href).toBe(before);
  });
});
