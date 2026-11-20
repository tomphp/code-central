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
    await renderLive(w, <SpecPane spec={spec} now={w.now()} select={() => {}} />);
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
    await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={select} />);
    const list = screen.getByRole("list", { name: "Tickets" });
    expect(list).toHaveTextContent("Accepted");
    await userEvent.click(within(list).getByRole("button", { name: /Add a page/ }));
    expect(select).toHaveBeenCalledWith(expect.stringMatching(/^acme-/));
  });

  describe("Ticket graph", () => {
    async function specWithEdge() {
      const w = await newWorld();
      const s = scenario(w);
      await s.project("acme", "Acme");
      const id = await s.spec("acme", { title: "Sign-in" });
      const store = await s.ticket("acme", { title: "Add a store", spec: id, state: "Done" });
      const page = await s.ticket("acme", { title: "Add a page", spec: id });
      const dropped = await s.ticket("acme", { title: "Try a cache", spec: id, state: "Cancelled" });
      await s.addBlocker("acme", page, store);
      return { w, s, id, ids: { store, page, dropped } };
    }

    it("shows the graph above the Tickets list, with the edge, progress, State chip and no type line", async () => {
      const { w, id, ids } = await specWithEdge();
      await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={() => {}} />);
      const graph = screen.getByRole("list", { name: "Ticket graph" });
      const tickets = screen.getByRole("list", { name: "Tickets" });
      expect(graph.compareDocumentPosition(tickets) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      const node = (n: string) => graph.querySelector<HTMLElement>(`[data-id="${n}"]`)!;
      expect([...graph.querySelectorAll<SVGElement>("[data-edge]")].map((e) => e.dataset.edge)).toEqual([`${ids.store}>${ids.page}`]);
      expect(node(ids.store).dataset.progress).toBe("done");
      expect(node(ids.page).dataset.progress).toBe("queued");
      expect(node(ids.page)).toHaveTextContent("Ready");
      expect(node(ids.page)).not.toHaveTextContent("ticket");
    });

    it("draws Done and Cancelled Tickets as nodes, the Cancelled one struck through", async () => {
      const { w, id, ids } = await specWithEdge();
      await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={() => {}} />);
      const graph = screen.getByRole("list", { name: "Ticket graph" });
      const node = (n: string) => graph.querySelector<HTMLElement>(`[data-id="${n}"]`)!;
      expect(node(ids.store)).not.toBeNull();
      expect(node(ids.dropped).querySelector(".cancelled")).not.toBeNull();
      expect(node(ids.store).querySelector(".cancelled")).toBeNull();
    });

    it("draws no node or edge for a Blocker on another Spec", async () => {
      const { w, s, id, ids } = await specWithEdge();
      const other = await s.spec("acme", { title: "Billing" });
      const away = await s.ticket("acme", { title: "Bill", spec: other });
      await s.addBlocker("acme", ids.dropped, away);
      await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={() => {}} />);
      const graph = screen.getByRole("list", { name: "Ticket graph" });
      expect(graph.querySelector(`[data-id="${away}"]`)).toBeNull();
      expect(graph.querySelectorAll("[data-edge]")).toHaveLength(1);
    });

    it("selects a node's Ticket on click and marks it current", async () => {
      const { w, id, ids } = await specWithEdge();
      const select = vi.fn();
      const spec = await w.api.getSpec("acme", id);
      const { rerender } = await renderLive(w, <SpecPane spec={spec} now={w.now()} select={select} />);
      await userEvent.click(within(screen.getByRole("list", { name: "Ticket graph" })).getByRole("button", { name: /Add a page/ }));
      expect(select).toHaveBeenCalledWith(ids.page);
      rerender(<SpecPane spec={spec} now={w.now()} ticket={ids.page} select={select} />);
      expect(within(screen.getByRole("list", { name: "Ticket graph" })).getByRole("button", { name: /Add a page/ })).toHaveAttribute("aria-current", "true");
    });

    it("shows no graph while no Ticket blocks another", async () => {
      const w = await newWorld();
      const s = scenario(w);
      await s.project("acme", "Acme");
      const id = await s.spec("acme", { title: "Sign-in" });
      await s.ticket("acme", { title: "One", spec: id });
      await s.ticket("acme", { title: "Two", spec: id });
      await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={() => {}} />);
      expect(screen.queryByRole("list", { name: "Ticket graph" })).toBeNull();
      expect(screen.getByRole("list", { name: "Tickets" })).toBeInTheDocument();
    });

    it("lists no Tickets and no graph for a Long-lived Spec, but keeps the CI sums", async () => {
      const w = await newWorld();
      const s = scenario(w);
      await s.project("acme", "Acme");
      const id = await s.spec("acme", { title: "Chores", labels: ["long-lived"] });
      const a = await s.ticket("acme", { title: "Sweep", spec: id, state: "Accepted" });
      const b = await s.ticket("acme", { title: "Mop", spec: id });
      await s.addBlocker("acme", b, a);
      await s.pushed("acme", a);
      await s.ci("acme", a, "passed");
      await renderLive(w, <SpecPane spec={await w.api.getSpec("acme", id)} now={w.now()} select={() => {}} />);
      expect(screen.queryByRole("list", { name: "Ticket graph" })).toBeNull();
      expect(screen.queryByRole("list", { name: "Tickets" })).toBeNull();
      expect(screen.queryByText("No Tickets yet.")).toBeNull();
      expect(screen.getByRole("group", { name: "CI results" })).toHaveTextContent("1 passed");
    });
  });
});

describe("RequirementPane", () => {
  it("shows the Stage strip and no Build list before any implementation Ticket", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    const { spec } = await w.api.submitRequirement("acme", "Export a Spec");
    const read = await w.api.getSpec("acme", spec.id);
    await renderLive(w, <RequirementPane spec={{ ...read, requirement: read.requirement! }} now={w.now()} select={() => {}} specLink={link} />);
    expect(within(screen.getByRole("list", { name: "Stages" })).getAllByRole("listitem")).toHaveLength(4);
    expect(screen.queryByRole("list", { name: "Build" })).toBeNull();
  });
});
