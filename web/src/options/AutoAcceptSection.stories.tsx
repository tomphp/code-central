import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutoAcceptSection } from "./AutoAcceptSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/AutoAcceptSection",
  component: AutoAcceptSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof AutoAcceptSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const SomeOn: Story = {
  args: {
    project: storyProject({
      auto_accept: { grilling: true, spec: false, tickets: false, implementation: true, merge: false },
    }),
  },
};
