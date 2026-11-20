// The Ticket graph without the Map's extras, as the Spec view and a
// Requirement's view draw it: rendered on a WASM Core world's Map, whose
// rows carry their Blockers, read through the list it renders.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import { renderLive } from "../worldTesting";
import { TicketGraph } from "./TicketGraph";

async function graph(selected?: (ids: Record<string, string>) => string) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const { spec } = await w.api.submitMap("acme", "Offline mode", true);
  const store = await s.ticket("acme", { title: "Which store", spec: spec.id, state: "Done", labels: ["wayfinder:grilling"] });
  const cache = await s.ticket("acme", { title: "Try a cache", spec: spec.id, state: "Cancelled", labels: ["prototype"] });
  const sync = await s.ticket("acme", { title: "How to sync", spec: spec.id, labels: ["wayfinder:grilling"] });
  await s.addBlocker("acme", sync, store);
  const ids = { store, cache, sync };
  const { tickets = [] } = await w.api.getSpec("acme", spec.id);
  const select = vi.fn();
  await renderLive(w, <TicketGraph tickets={tickets} label="Ticket graph" ticket={selected?.(ids)} select={select} />);
  const list = screen.getByRole("list", { name: "Ticket graph" });
  const node = (id: string) => list.querySelector<HTMLElement>(`[data-id="${id}"]`)!;
  return { ids, list, node, select };
}

describe("TicketGraph", () => {
  it("draws each Ticket as a node with its progress, State chip and title but no type line, and each Blocker as an edge", async () => {
    const { ids, list, node } = await graph();

    expect(within(list).getAllByRole("listitem").map((n) => n.dataset.id)).toEqual(expect.arrayContaining(Object.values(ids)));
    expect(node(ids.store).dataset.progress).toBe("done");
    expect(node(ids.sync).dataset.progress).toBe("queued");
    expect(node(ids.store)).toHaveTextContent("Done");
    expect(node(ids.store)).toHaveTextContent("Which store");
    expect(node(ids.store)).not.toHaveTextContent("grilling");
    expect([...list.querySelectorAll<SVGElement>("[data-edge]")].map((e) => e.dataset.edge)).toEqual([`${ids.store}>${ids.sync}`]);
  });

  it("strikes through a Cancelled Ticket", async () => {
    const { ids, node } = await graph();

    expect(node(ids.cache).querySelector(".cancelled")).not.toBeNull();
    expect(node(ids.store).querySelector(".cancelled")).toBeNull();
  });

  it("has no fog node and no Start", async () => {
    const { list } = await graph();

    expect(list.querySelector("[data-fog]")).toBeNull();
    expect(within(list).queryByRole("button", { name: "Not yet specified" })).toBeNull();
    expect(within(list).queryByRole("button", { name: /^Start/ })).toBeNull();
  });

  it("selects a node's Ticket on click and marks the selected node current", async () => {
    const { ids, node, select } = await graph((ids) => ids.sync);

    expect(within(node(ids.sync)).getByRole("button", { name: /How to sync/ })).toHaveAttribute("aria-current", "true");
    expect(within(node(ids.store)).getByRole("button", { name: /Which store/ })).not.toHaveAttribute("aria-current");
    await userEvent.click(within(node(ids.store)).getByRole("button", { name: /Which store/ }));
    expect(select).toHaveBeenCalledWith(ids.store);
  });
});
