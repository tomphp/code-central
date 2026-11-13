// The Tickets the Ticket row's and card's stories show, read from the fake
// API as the Inbox reads them: one for each part a row or card shows only
// sometimes. Only stories import this.
import { FakeApi } from "../api/fake";
import { STATES } from "../api/types";
import type { ProjectTicket } from "../live";
import type { RowLink } from "./TicketLine";

// StoryTickets are the stories' Tickets: one with nothing optional, one In
// Progress with a latest Session Event, one linked, Blocked and waiting for
// its Spec, and a Stage Ticket. now is the fake API's time.
export interface StoryTickets {
  now: Date;
  plain: ProjectTicket;
  working: ProjectTicket;
  linked: ProjectTicket;
  stage: ProjectTicket;
}

// storyTickets seeds a fake API and reads its Tickets back, as a story's
// loader.
export async function storyTickets(): Promise<StoryTickets> {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const ago = (minutes: number) => new Date(fake.now.getTime() - minutes * 60_000).toISOString();
  const { id, title } = fake.addSpec("acme", { title: "Sign-in" });
  const spec = { id, title };
  fake.addTicket("acme", { title: "Add a sign-in page", state: "Ready", spec });
  fake.addTicket("acme", {
    title: "Rate-limit the sign-in endpoint",
    state: "In Progress",
    spec,
    claims: [{ number: 1, session_id: "s-1", started: ago(30), last_activity: ago(1) }],
  });
  fake.postSessionEvents("s-1", [{ time: ago(2), kind: "tool_call", tool: "Read", text: "Read: core/core.go" }]);
  fake.addTicket("acme", {
    title: "Sync sign-ins with GitHub",
    state: "Accepted",
    spec,
    blocked: true,
    waiting_for_spec: true,
    link: { integration: "GitHub", id: "#12", url: "https://github.com/acme/widgets/issues/12", external_state: "open", detached: false, outbound: false },
  });
  fake.addTicket("acme", { title: "Grilling: Sign-in", state: "Needs Answer", spec, labels: ["stage:grilling"] });
  const rows = await fake.findTickets("acme", { state: [...STATES] });
  const [plain, working, linked, stage] = rows.map((r) => ({ ...r, project: "acme" }));
  return { now: fake.now, plain, working, linked, stage };
}

// storyLink is a row's or card's link that goes nowhere.
export const storyLink = (id: string): RowLink => ({ href: `#${id}`, onClick: (e) => e.preventDefault(), "data-id": id });
