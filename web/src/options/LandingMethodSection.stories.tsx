import type { Meta, StoryObj } from "@storybook/react-vite";
import { LandingMethodSection } from "./LandingMethodSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/LandingMethodSection",
  render: (_args, { loaded }) => <LandingMethodSection {...loaded.world} />,
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
export const Archived: Story = { ...optionsWorld(async (s) => s.archive("acme")) };
