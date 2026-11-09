// Map tests: the real App against FakeApi. "New Map", the Map's row in
// the Requirements view and the Map pane.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });
const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const list = () => screen.getByRole("main", { name: "Tickets" });
const params = () => new URLSearchParams(window.location.search);
const row = (id: string) => list().querySelector<HTMLElement>(`a[data-id="${id}"]`)!;

describe("New Map", () => {
  it("sits beside New requirement, submits the same free-text dialog and selects the new Map", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    renderApp(fake);

    const newMap = await within(nav()).findByRole("button", { name: "New Map" });
    expect(newMap.parentElement).toBe(within(nav()).getByRole("button", { name: "New requirement" }).parentElement);
    await userEvent.click(newMap);
    const modal = screen.getByRole("dialog", { name: "New Map" });
    expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
    expect(within(modal).getByRole("button", { name: "Submit map" })).toBeDisabled();
    await userEvent.type(within(modal).getByRole("textbox", { name: "Map" }), "Offline mode{Enter}Works on a plane.");
    await userEvent.click(within(modal).getByRole("button", { name: "Submit map" }));

    expect(fake.calls).toContain("submitMap");
    expect(fake.calls).not.toContain("submitRequirement");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(params().get("requirement")).toBe("acme-1");
    const pane = await within(detail()).findByRole("region", { name: "Map" });
    expect(within(pane).getByRole("heading", { name: "Offline mode", level: 2 })).toBeInTheDocument();
    expect(pane).toHaveTextContent("0/1 decided");
    expect(within(within(pane).getByRole("list", { name: "Map Tickets" })).getByRole("listitem")).toHaveTextContent(
      "acme-2 Charting: Offline mode chart Ready",
    );
    expect(row("acme-1")).toHaveTextContent("0/1 decided");
  });

  it("opens a Map Ticket from the Map pane", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitMap("acme", "Offline mode"); // acme-1, Charting acme-2
    renderApp(fake, "/?view=inbox&list=requirements&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Map" });
    await userEvent.click(within(pane).getByRole("button", { name: /Charting: Offline mode/ }));
    expect(params().get("ticket")).toBe("acme-2");
    expect(await within(detail()).findByRole("heading", { name: "Charting: Offline mode" })).toBeInTheDocument();
  });
});

describe("a Map's row in the Requirements view", () => {
  it("shows the decided count in place of the strip, and what needs you only when something does", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const quiet = await fake.submitMap("acme", "Offline mode"); // acme-1, Charting acme-2
    const busy = await fake.submitMap("acme", "Sync"); // acme-3, Charting acme-4
    const spec = { id: busy.spec.id, title: "Sync" };
    fake.updateTicket(busy.charting.id, { state: "Done" });
    fake.addTicket("acme", { title: "Which store", state: "In Review", spec, labels: ["wayfinder:grilling"] }); // acme-5
    fake.addTicket("acme", { title: "Try a cache", state: "In Review", spec, labels: ["prototype"] }); // acme-6
    fake.addTicket("acme", { title: "Dropped", state: "Cancelled", spec, labels: ["wayfinder:task"] }); // acme-7
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-5" }, { id: "acme-6" }] }]);
    const req = await fake.submitRequirement("acme", "Export a Spec"); // acme-8
    renderApp(fake);

    await userEvent.click(await within(nav()).findByRole("button", { name: "Requirements 3" }));
    const ids = () => [...list().querySelectorAll<HTMLElement>("a[data-id]")].map((a) => a.dataset.id);
    await waitFor(() => expect(ids()).toEqual([req.spec.id, busy.spec.id, quiet.spec.id]));

    expect(within(row(busy.spec.id)).queryByRole("list", { name: "Stages" })).toBeNull();
    expect(row(busy.spec.id)).toHaveTextContent("1/2 decided · 2 needs you");
    expect(within(row(busy.spec.id)).getByLabelText("Needs you")).toBeInTheDocument();
    expect(row(quiet.spec.id)).toHaveTextContent("0/1 decided");
    expect(row(quiet.spec.id)).not.toHaveTextContent("needs you");
    expect(within(row(quiet.spec.id)).queryByLabelText("Needs you")).toBeNull();
    expect(within(row(req.spec.id)).getByRole("list", { name: "Stages" })).toBeInTheDocument();
  });
});
