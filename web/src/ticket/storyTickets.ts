// The Tickets the Ticket row's and card's stories show, read from a WASM
// Core world as the Inbox reads them: one for each part a row or card shows
// only sometimes. Only stories import this.
import { newWorld } from "../api/core/core";
import { SESSION } from "../api/driver";
import { scenario } from "../api/scenario";
import { STATES } from "../api/types";
import type { ProjectTicket } from "../live";
import type { RowLink } from "./TicketLine";

// StoryTickets are the stories' Tickets: one with nothing optional, one In
// Progress with a latest Session Event, one linked, Blocked and waiting for
// its Spec, and a Stage Ticket. now is the world's time.
export interface StoryTickets {
  now: Date;
  plain: ProjectTicket;
  working: ProjectTicket;
  linked: ProjectTicket;
  stage: ProjectTicket;
}

const MIN = 60_000;

// storyTickets builds a world with the scenario builder and reads its
// Tickets back, as a story's loader. The Tickets are plain data, so the
// world is dropped once they are read.
export async function storyTickets(): Promise<StoryTickets> {
  const w = await newWorld();
  try {
    const s = scenario(w);
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { title: "Sign-in" });
    const plain = await s.ticket("acme", { title: "Add a sign-in page", spec });
    const working = await s.ticket("acme", { title: "Rate-limit the sign-in endpoint", spec, state: "In Progress" });
    w.advance(28 * MIN);
    await s.postSessionEvents(SESSION,[{ time: w.now().toISOString(), kind: "tool_call", tool: "Read", text: "Read: core/core.go" }]);
    w.advance(2 * MIN);
    const linked = await s.linkedTicket("acme", {
      title: "Sync sign-ins with GitHub",
      spec,
      state: "Ready",
      link: { integration: "GitHub", id: "#12", url: "https://github.com/acme/widgets/issues/12" },
    });
    await s.accepted("acme", linked);
    await s.addBlocker("acme", linked, plain);
    const stage = await s.ticket("acme", { title: "Grilling: Sign-in", spec, state: "Needs Answer", labels: ["stage:grilling"] });

    const rows = await w.api.findTickets("acme", { state: [...STATES] });
    const row = (id: string): ProjectTicket => ({ ...rows.find((r) => r.id === id)!, project: "acme" });
    return { now: w.now(), plain: row(plain), working: row(working), linked: row(linked), stage: row(stage) };
  } finally {
    w.drop();
  }
}

// storyLink is a row's or card's link that goes nowhere.
export const storyLink = (id: string): RowLink => ({ href: `#${id}`, onClick: (e) => e.preventDefault(), "data-id": id });
