import type { Meta, StoryObj } from "@storybook/react-vite";
import { WaitingBadge } from "./WaitingBadge";

const meta = { title: "Ticket/WaitingBadge", component: WaitingBadge } satisfies Meta<typeof WaitingBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Waiting: Story = { args: { waiting: true } };
// Shows nothing.
export const NotWaiting: Story = { args: { waiting: false } };
