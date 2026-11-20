// The Ticket graph without the Map's extras, as the Spec view and a
// Requirement's view draw it, on the WASM Core: each story reads a Map's
// rows, which carry their Blockers. The Map's own graph is in Map/MapBody.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import type { Scenario } from "../api/scenario";
import type { TicketRow } from "../api/types";
import { TicketGraph } from "./TicketGraph";

const meta = { title: "Graph/TicketGraph" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

interface Loaded {
  tickets: TicketRow[];
  selected?: string;
}

const graph = (build: (s: Scenario, spec: string) => Promise<string | void>): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    const { spec } = await w.api.submitMap("acme", "Offline mode", true);
    const selected = (await build(s, spec.id)) ?? undefined;
    const { tickets = [] } = await w.api.getSpec("acme", spec.id);
    return { tickets, selected };
  }),
  render: (_, { loaded }) => {
    const { tickets, selected } = loaded.world as Loaded;
    return <TicketGraph tickets={tickets} label="Ticket graph" ticket={selected} select={() => {}} />;
  },
});

// A chain of Blockers in every progress colour, a Cancelled Ticket struck
// through, and the running one selected.
export const Chain = graph(async (s, spec) => {
  const store = await s.ticket("acme", { title: "Which store", spec, state: "Done" });
  const cache = await s.ticket("acme", { title: "Try a cache", spec, state: "In Progress" });
  const review = await s.ticket("acme", { title: "Review the cache", spec, state: "In Review" });
  const sync = await s.ticket("acme", { title: "How to sync", spec });
  const later = await s.ticket("acme", { title: "Sync later", spec, state: "Draft" });
  await s.ticket("acme", { title: "Drop offline", spec, state: "Cancelled" });
  await s.addBlocker("acme", cache, store);
  await s.addBlocker("acme", review, cache);
  await s.addBlocker("acme", sync, store);
  await s.addBlocker("acme", later, sync);
  return cache;
});

// So many Tickets in a row that the graph scrolls sideways in its box.
export const Wide = graph(async (s, spec) => {
  let blocker: string | undefined;
  for (let i = 1; i <= 9; i++) {
    const t = await s.ticket("acme", { title: `Step ${i}`, spec });
    if (blocker) await s.addBlocker("acme", t, blocker);
    blocker = t;
  }
});
