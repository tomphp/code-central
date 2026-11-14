import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { KindBadge } from "../controls/KindBadge";
import { ShortId } from "../inbox/ShortId";
import { Needing } from "../needsme/fixture";
import { TicketLine } from "./TicketLine";
import { storyLink, storyTickets, type StoryTickets } from "./storyTickets";

const meta = { title: "Ticket/TicketLine", loaders: [async () => ({ data: await storyTickets() })] } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

// row renders a story from the fake API's Tickets.
const row = (draw: (d: StoryTickets) => ReactNode): Story => ({
  render: (_, { loaded }) => <>{draw(loaded.data as StoryTickets)}</>,
});

// The ID and title alone.
export const Plain = row(({ plain }) => <TicketLine t={plain} link={storyLink(plain.id)} />);
// A Stage Ticket among its Requirement's Tickets.
export const WithStage = row(({ stage }) => <TicketLine t={stage} link={storyLink(stage.id)} badge="Grilling" />);
// In a list of several States.
export const WithStateChip = row(({ plain }) => <TicketLine t={plain} link={storyLink(plain.id)} chip />);
// Linked to a GitHub issue, and waiting for its Spec to land.
export const LinkedAndWaiting = row(({ linked }) => <TicketLine t={linked} link={storyLink(linked.id)} chip />);
export const NeedsYou = row(({ stage }) => (
  <Needing tickets={[stage]} reasons={{ [stage.id]: "needs_answer" }}>
    <TicketLine t={stage} link={storyLink(stage.id)} />
  </Needing>
));
// An In Progress Ticket's latest Session Event on its second line.
export const LatestEvent = row(({ now, working }) => <TicketLine t={working} link={storyLink(working.id)} now={now} />);
// The list's own second line: All Tickets names the Spec.
export const WithDetail = row(({ plain }) => (
  <TicketLine
    t={plain}
    link={storyLink(plain.id)}
    chip
    detail={
      <>
        <KindBadge kind="Spec" /> <ShortId id={plain.spec} /> Sign-in
      </>
    }
  />
));
// A Draft's row.
export const Quiet = row(({ plain }) => <TicketLine t={plain} link={storyLink(plain.id)} quiet />);
// Every optional part at once, with a badge of the list's own.
export const Everything = row(({ now, linked }) => (
  <Needing tickets={[linked]} reasons={{ [linked.id]: "in_review" }}>
    <TicketLine t={linked} link={storyLink(linked.id)} badge="Prototype" chip now={now}>
      <span className="muted small">3h</span>
    </TicketLine>
  </Needing>
));
