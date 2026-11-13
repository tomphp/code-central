import type { Meta, StoryObj } from "@storybook/react-vite";
import { ModelsSection } from "./ModelsSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/ModelsSection",
  component: ModelsSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof ModelsSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
