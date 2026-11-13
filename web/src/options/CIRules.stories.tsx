import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIRules } from "./CIRules";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/CIRules",
  component: CIRules,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof CIRules>;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithRules: Story = {};
export const NoRules: Story = {
  args: {
    project: storyProject({ ci: { ...storyProject().ci, rules: [] } }),
  },
};
