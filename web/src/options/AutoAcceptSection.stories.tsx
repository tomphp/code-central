import type { Meta, StoryObj } from "@storybook/react-vite";
import { AutoAcceptSection } from "./AutoAcceptSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/AutoAcceptSection",
  render: (_args, { loaded }) => <AutoAcceptSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = { ...optionsWorld() };
export const SomeOn: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setAutoAccept("acme", { grilling: true, implementation: true });
  }),
};
