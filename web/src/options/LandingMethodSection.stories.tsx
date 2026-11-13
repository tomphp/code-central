import type { Meta, StoryObj } from "@storybook/react-vite";
import { LandingMethodSection } from "./LandingMethodSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/LandingMethodSection",
  component: LandingMethodSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof LandingMethodSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Merge: Story = {};
export const PullRequest: Story = {
  args: {
    project: storyProject({
      landing: {
        ...storyProject().landing,
        method: "pull_request",
        code_host: "github",
        repository: "acme/web",
        watcher: { state: "watching" },
        open_pull_requests: [],
      },
    }),
  },
};
export const Archived: Story = { args: { project: storyProject({ archived: true }) } };
