import type { Meta, StoryObj } from "@storybook/react-vite";
import { FocusSection } from "./FocusSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/FocusSection",
  component: FocusSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof FocusSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
