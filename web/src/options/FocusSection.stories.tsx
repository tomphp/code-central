import type { Meta, StoryObj } from "@storybook/react-vite";
import { FocusSection } from "./FocusSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/FocusSection",
  render: (_args, { loaded }) => <FocusSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { ...optionsWorld() };
