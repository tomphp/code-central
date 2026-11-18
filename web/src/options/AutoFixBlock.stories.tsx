import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutoFixBlock } from "./AutoFixBlock";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/AutoFixBlock",
  render: (_args, { loaded }) => <AutoFixBlock {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Off: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setCITracking("acme", { tracking: true, ignored_workflows: [] });
  }),
};
export const On: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setCITracking("acme", { tracking: true, ignored_workflows: [] });
    await w.api.setAutoFix("acme", { on: true, fix_instructions: "Fix it the way the failure needs.", limit: 5 });
  }),
};
