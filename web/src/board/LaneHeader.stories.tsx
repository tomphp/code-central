import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import { STATES } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { Needing } from "../needsme/fixture";
import { storyLink } from "../ticket/storyTickets";
import { LaneHeader } from "./LaneHeader";

// On the WASM Core: a Spec with a Ticket in each of four States, read back
// as the Inbox reads them.
interface StoryTickets {
  all: ProjectTicket[];
  stage: ProjectTicket;
}

const meta = {
  title: "Board/LaneHeader",
  ...storyWorld(async (s, w): Promise<StoryTickets> => {
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { title: "Sign-in" });
    await s.ticket("acme", { title: "Add a sign-in page", spec });
    await s.ticket("acme", { title: "Rate-limit the sign-in endpoint", spec, state: "In Progress" });
    await s.ticket("acme", { title: "Sync sign-ins with GitHub", spec, state: "Accepted" });
    await s.ticket("acme", { title: "Grilling: Sign-in", spec, state: "Needs Answer", labels: ["stage:grilling"] });
    const rows = await w.api.findTickets("acme", { state: [...STATES] });
    const all = rows.map((r) => ({ ...r, project: "acme" }));
    return { all, stage: all[3] };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

// spec is the row of the world's one Spec, the first ID Core hands out.
const spec = (title: string, labels: string[] = []): ProjectSpec => ({
  id: "acme-1",
  project: "acme",
  title,
  labels,
  tags: {},
  state_summary: { Ready: 1, "In Progress": 1, Accepted: 1, "Needs Answer": 1 },
  finished: false,
  updated: "",
});

// header renders a story from the world's Tickets, as wide as a Board, with
// Needs Me's reasons for them.
const header = (draw: (d: StoryTickets, all: ProjectTicket[]) => ReactNode, reasons: (d: StoryTickets) => Record<string, "needs_answer"> = () => ({})): Story => ({
  render: (_, { loaded }) => {
    const d = loaded.world as StoryTickets;
    return (
      <Needing tickets={d.all} reasons={reasons(d)}>
        <div style={{ width: 900 }}>{draw(d, d.all)}</div>
      </Needing>
    );
  },
});

export const Open = header((_, all) => (
  <LaneHeader id="acme-1" spec={spec("Sign-in")} all={all} shown={3} link={storyLink("acme-1")} collapsed={false} onToggle={() => {}} />
));
export const Folded = header((_, all) => (
  <LaneHeader id="acme-1" spec={spec("Sign-in")} all={all} shown={3} link={storyLink("acme-1")} collapsed onToggle={() => {}} />
));
// One of its Tickets needs the human: the dot, a left border, and the need as the headline.
export const NeedsYou = header(
  (_, all) => <LaneHeader id="acme-1" spec={spec("Sign-in")} all={all} shown={3} link={storyLink("acme-1")} collapsed={false} onToggle={() => {}} />,
  ({ stage }) => ({ [stage.id]: "needs_answer" }),
);
// A long title gives way to the headline and the count.
export const LongTitle = header((_, all) => (
  <LaneHeader
    id="acme-1"
    spec={spec("Search across Tickets and Specs with filters that remember what you last picked, per Project and per browser", ["long-lived"])}
    all={all}
    shown={4}
    link={storyLink("acme-1")}
    collapsed={false}
    onToggle={() => {}}
  />
));
