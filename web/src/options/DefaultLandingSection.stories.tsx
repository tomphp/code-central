import type { Meta, StoryObj } from "@storybook/react-vite";
import { DefaultLandingSection } from "./DefaultLandingSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/DefaultLandingSection",
  render: (_args, { loaded }) => <DefaultLandingSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Merge: Story = { ...optionsWorld() };
export const PullRequest: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
    await s.reportPullRequestWatcher("acme", { state: "watching" });
  }),
};
export const PullRequestOnAzureRepos: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "https://dev.azure.com/acme/shop/_git/web");
    await w.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
  }),
};
// An Azure Repos Repository's Own Landing of Pull Request, under a Default
// Landing of Merge, brings in the PAT variable and the Pull Request Watcher.
export const OwnPullRequestOnAzureRepos: Story = {
  ...optionsWorld(async (s, w) => {
    await s.reportPushRemote("acme", "git@github.com:acme/web.git");
    await w.api.addRepository("acme", { name: "infra", clone_url: "https://dev.azure.com/acme/ops/_git/infra", landing: "pull_request" });
  }),
};
export const Archived: Story = { ...optionsWorld(async (s) => s.archive("acme")) };
