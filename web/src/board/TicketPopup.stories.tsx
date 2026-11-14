// The Board's Ticket popup. On the WASM Core: acme's "Add a sign-in page"
// In Review on the Spec "Sign-in", read back and handed in as props. The
// api is passed because the Ticket pane acts.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import { ApiError, type ProjectRow, type TicketView } from "../api/types";
import { TicketPopupView } from "./TicketPopup";

interface World {
  api: Api;
  now: Date;
  projects: ProjectRow[];
  ticket: TicketView;
}

const meta = {
  title: "Board/TicketPopup",
  ...storyWorld(async (s, w): Promise<World> => {
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { title: "Sign-in" });
    const id = await s.ticket("acme", { title: "Add a sign-in page", spec, description: "A page at /sign-in with email and password." });
    await s.inReview("acme", id);
    return { api: w.api, now: w.now(), projects: await w.api.listProjects(), ticket: await w.api.getTicket("acme", id) };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const popup = (read: (w: World) => { data?: TicketView; error?: ApiError }): Story => ({
  render: (_, { loaded }) => {
    const w = loaded.world as World;
    return (
      <TicketPopupView
        api={w.api}
        now={w.now}
        id={w.ticket.id}
        projects={w.projects}
        ticket={read(w)}
        onClose={() => {}}
        openSpec={() => {}}
        openTicket={() => {}}
      />
    );
  },
});

export const Ticket = popup((w) => ({ data: w.ticket }));
export const Loading = popup(() => ({}));
export const ReadFailed = popup(() => ({ error: new ApiError(404, "not_found", "no Ticket acme-2") }));
