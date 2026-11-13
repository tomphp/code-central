import type { Meta, StoryObj } from "@storybook/react-vite";
import { StageGuidesSection } from "./StageGuidesSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/StageGuidesSection",
  component: StageGuidesSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof StageGuidesSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const BothOn: Story = {
  args: {
    project: storyProject({
      stage_guides: { subagent_exploration: true, model_assignment: true },
    }),
  },
};
