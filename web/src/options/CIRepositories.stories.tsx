import type { Meta, StoryObj } from "@storybook/react-vite";
import { CIRepositories } from "./CIRepositories";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/CIRepositories",
  render: (_args, { loaded }) => <CIRepositories {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const WatchedAndNotWatched: Story = {
  ...optionsWorld(async (_s, w) => {
    await w.api.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    await w.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    await w.api.addRepository("acme", { name: "infra", clone_url: "https://dev.azure.com/acme/ops/_git/infra" });
    await w.api.addRepository("acme", { name: "docs", clone_url: "https://gitlab.com/acme/docs.git" });
  }),
};
