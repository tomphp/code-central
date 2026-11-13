import type { Meta, StoryObj } from "@storybook/react-vite";
import { FactoriesSection } from "./FactoriesSection";
import { storyProject } from "./storyFixtures";

const meta = {
  title: "Options/FactoriesSection",
  component: FactoriesSection,
  args: { now: new Date("2026-11-13T12:00:10Z") },
} satisfies Meta<typeof FactoriesSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Online: Story = { args: { project: storyProject() } };
export const NoFactories: Story = {
  args: {
    project: storyProject({
      factory_status: { status: "offline", factories: [] },
    }),
  },
};
