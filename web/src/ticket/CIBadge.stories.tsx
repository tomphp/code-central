import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIBadge } from "./CIBadge";

const meta = { title: "Ticket/CIBadge", component: CIBadge } satisfies Meta<typeof CIBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = { args: { result: "running" } };
export const Passed: Story = { args: { result: "passed" } };
export const Failed: Story = { args: { result: "failed" } };
export const Cancelled: Story = { args: { result: "cancelled" } };
// Shows nothing.
export const NoResult: Story = {};
