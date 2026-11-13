import type { ReactNode } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TicketCard } from "./TicketCard";
import { storyLink, storyTickets, type StoryTickets } from "./storyTickets";

const meta = { title: "Ticket/TicketCard", loaders: [async () => ({ data: await storyTickets() })] } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

// card renders a story from the fake API's Tickets, as wide as a Board column.
const card = (draw: (d: StoryTickets) => ReactNode): Story => ({
  render: (_, { loaded }) => <div style={{ width: 240 }}>{draw(loaded.data as StoryTickets)}</div>,
});

// The ID and title alone, as in a Spec's lane.
export const Plain = card(({ plain }) => <TicketCard t={plain} link={storyLink(plain.id)} />);
export const WithStage = card(({ stage }) => <TicketCard t={stage} link={storyLink(stage.id)} badge="Grilling" />);
export const Blocked = card(({ linked }) => <TicketCard t={linked} link={storyLink(linked.id)} />);
export const NeedsYou = card(({ stage }) => <TicketCard t={stage} link={storyLink(stage.id)} reason="needs_answer" />);
// Outside a lane, the card names its Spec.
export const WithSpec = card(({ plain }) => <TicketCard t={plain} link={storyLink(plain.id)} spec={{ kind: "Spec", title: "Sign-in" }} />);
// A card that just joined its column pulses.
export const Joined = card(({ plain }) => <TicketCard t={plain} link={storyLink(plain.id)} joined />);
export const Everything = card(({ linked }) => (
  <TicketCard t={linked} link={storyLink(linked.id)} badge="Prototype" reason="in_review" spec={{ kind: "Map", title: "Sign-in" }} />
));
