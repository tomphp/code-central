import type { Meta, StoryObj } from "@storybook/react-vite";
import { FilterButton } from "./FilterButton";

const meta = {
  title: "Inbox/FilterButton",
  component: FilterButton,
  args: { on: false, label: "Needs Me", count: 3, onClick: () => {} },
} satisfies Meta<typeof FilterButton>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Off: Story = {};

export const On: Story = { args: { on: true } };

// A loud count draws the eye when something is waiting.
export const Loud: Story = { args: { loud: true } };

// A State under the Views: indented.
export const Sub: Story = { args: { sub: true, label: "In Review" } };
