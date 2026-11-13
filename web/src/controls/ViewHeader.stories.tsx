import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHeader } from "./ViewHeader";

const meta = { title: "Controls/ViewHeader", component: ViewHeader } satisfies Meta<typeof ViewHeader>;
export default meta;
type Story = StoryObj<typeof meta>;

export const TitleOnly: Story = { args: { title: "Board" } };

export const WithSlots: Story = {
  args: {
    title: "Board",
    left: <span className="muted small">3 Tickets</span>,
    right: <button>+ New</button>,
  },
};
