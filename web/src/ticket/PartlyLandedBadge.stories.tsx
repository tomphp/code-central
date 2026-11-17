import type { Meta, StoryObj } from "@storybook/react-vite";
import { PartlyLandedBadge } from "./PartlyLandedBadge";

const meta = { title: "Ticket/PartlyLandedBadge", component: PartlyLandedBadge } satisfies Meta<typeof PartlyLandedBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const PartlyLanded: Story = { args: { partly: true } };
// Shows nothing.
export const NotPartlyLanded: Story = { args: { partly: false } };
