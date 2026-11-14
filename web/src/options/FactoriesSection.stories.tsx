import type { Meta, StoryObj } from "@storybook/react-vite";
import { FactoriesSection } from "./FactoriesSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/FactoriesSection",
  render: (_args, { loaded }) => <FactoriesSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Online: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportHeartbeat("acme", { factory: "factory-1", host: "dev-box", state: "online", interval_seconds: 30 });
    w.advance(10_000);
  }),
};
export const NoFactories: Story = { ...optionsWorld() };
