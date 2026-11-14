import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ProjectSpec } from "../live";
import { Needing } from "../needsme/fixture";
import { storyLink, storyTickets, type StoryTickets } from "../ticket/storyTickets";
import { LaneHeader } from "./LaneHeader";

const meta = { title: "Board/LaneHeader", loaders: [async () => ({ data: await storyTickets() })] } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

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

// header renders a story from the fake API's Tickets, as wide as a Board,
// with Needs Me's reasons for them.
const header = (draw: (d: StoryTickets, all: StoryTickets["plain"][]) => ReactNode, reasons: (d: StoryTickets) => Record<string, "needs_answer"> = () => ({})): Story => ({
  render: (_, { loaded }) => {
    const d = loaded.data as StoryTickets;
    const all = [d.plain, d.working, d.linked, d.stage];
    return (
      <Needing tickets={all} reasons={reasons(d)}>
        <div style={{ width: 900 }}>{draw(d, all)}</div>
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
