// The Focus popup in each mode. On the WASM Core: acme with a Round of two
// Questions waiting on "Pick a colour", "Flaky build" Failed and "Login
// page" In Review, read back as the Inbox reads them and handed in as
// props with Needs Me over them. The api is passed because the popup acts
// on its Focus Item.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import { loadInbox, type InboxData } from "../live";
import { NeedsMeProvider } from "../needsme/context";
import { needsMeOf } from "../needsme/model";
import { FocusPopup } from "./FocusPopup";

interface World {
  api: Api;
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
    return { api: w.api, now: w.now(), data: await loadInbox(w.api), colour };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const props = ({ api, now, data }: World) => ({ api, now, data, version: 0, onClose: () => {}, navigate: () => {} });

// Ticket mode: scoped to the one Ticket, with no Skip, N left or Open in Inbox.
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

// Queue mode: every Focus Item in the Project, with Skip, N left and Open in Inbox.
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
