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
export const Archived: Story = { ...optionsWorld(async (s) => s.archive("acme")) };
