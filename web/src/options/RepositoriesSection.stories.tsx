import type { Meta, StoryObj } from "@storybook/react-vite";
import { RepositoriesSection } from "./RepositoriesSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/RepositoriesSection",
  render: (_args, { loaded }) => <RepositoriesSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const HomeAlone: Story = { ...optionsWorld() };
export const SeveralRepositories: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git", local_path: "/src/web" });
    await w.api.addRepository("acme", { name: "api", clone_url: "https://dev.azure.com/acme/shop/_git/api", base: "develop" });
  }),
};
