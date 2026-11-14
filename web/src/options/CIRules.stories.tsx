import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIRules } from "./CIRules";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/CIRules",
  render: (_args, { loaded }) => <CIRules {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithRules: Story = {
  ...optionsWorld(async (s, w) => {
    await s.trackCI("acme");
    await w.api.addCIRule("acme", { workflow: "ci.yml", branch: "main", state: "Draft" });
  }),
};
export const NoRules: Story = { ...optionsWorld(async (s) => s.trackCI("acme")) };
