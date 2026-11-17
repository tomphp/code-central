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
export const OwnLandingsAndCI: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "");
    await w.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git", landing: "pull_request", push_remote: "upstream" });
    await w.api.addRepository("acme", { name: "infra", clone_url: "https://dev.azure.com/acme/ops/_git/infra" });
    await w.api.addRepository("acme", { name: "docs", clone_url: "https://gitlab.com/acme/docs.git" });
  }),
};
export const LocalOnlyHomeUnderPullRequest: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await w.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
    await s.reportPushRemote("acme", "");
  }),
};
export const EveryRepositoryLocalOnly: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "");
    await w.api.setLandingMethod("acme", { method: "merge", push: true, remote: "origin" });
    await w.api.addRepository("acme", { name: "docs", local_path: "/src/docs" });
  }),
};
export const CloneStatus: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/acme.git");
    await w.api.addRepository("acme", { name: "web", clone_url: "git@github.com:acme/web.git" });
    await w.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    await s.clones("acme", [
      { name: "acme", path: "/src/acme", state: "ready" },
      { name: "web", path: "/src/web", state: "ready" },
      { name: "api", path: "/src/api", state: "missing", problem: "/src/api's remote origin is git@github.com:acme/other.git, not git@github.com:acme/api.git" },
    ]);
  }),
};
