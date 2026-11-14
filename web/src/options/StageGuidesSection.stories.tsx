import type { Meta, StoryObj } from "@storybook/react-vite";
import { StageGuidesSection } from "./StageGuidesSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/StageGuidesSection",
  render: (_args, { loaded }) => <StageGuidesSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { ...optionsWorld() };
export const BothOn: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setSubagentExploration("acme", true);
    await w.api.setModelAssignment("acme", true);
  }),
};
