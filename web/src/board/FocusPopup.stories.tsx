// The Focus popup in each mode. On the WASM Core: acme with a Round of two
// Questions waiting on "Pick a colour", "Flaky build" Failed and "Login
// page" In Review, read back as the Inbox reads them and handed in as
// props with Needs Me over them. The popup acts on
// its Focus Item through the story's <Live>.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { storyWorld } from "../api/core/story";
import { loadInbox, type InboxData } from "../live";
import { NeedsMeProvider } from "../needsme/context";
import { needsMeOf } from "../needsme/model";
import { FocusPopup } from "./FocusPopup";

interface World {
  now: Date;
  data: InboxData;
  colour: string;
}

const meta = {
  title: "Board/FocusPopup",
  ...storyWorld(async (s, w): Promise<World> => {
    await w.api.setOptions({ focus_clear: "all_clear" });
    await s.project("acme", "Acme");
    const colour = await s.ticket("acme", { title: "Pick a colour" });
    await s.askRound("acme", colour, [
      { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
      { title: "Which size?", body: "S or M" },
    ]);
    await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    return { now: w.now(), data: await loadInbox(w.api), colour };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const props = ({ now, data }: World) => ({ now, data, onClose: () => {}, navigate: () => {} });

// Ticket mode: scoped to the one Ticket, with no Skip or N left, and its title linking to the Ticket.
export const TicketMode: Story = {
  render: (_, { loaded }) => {
    const w = loaded.world as World;
    return (
      <NeedsMeProvider value={needsMeOf(w.data)}>
        <FocusPopup {...props(w)} mode="ticket" id={w.colour} />
      </NeedsMeProvider>
    );
  },
};

// Queue mode: every Focus Item in the Project, with Skip and N left, and its title linking to the Ticket.
export const QueueMode: Story = {
  render: (_, { loaded }) => {
    const w = loaded.world as World;
    return (
      <NeedsMeProvider value={needsMeOf(w.data)}>
        <FocusPopup {...props(w)} mode="queue" project="acme" />
      </NeedsMeProvider>
    );
  },
};
