import type { Meta, StoryObj } from "@storybook/react-vite";
import { GitHubIssuesSection } from "./GitHubIssuesSection";
import { optionsWorld } from "./storyFixtures";

const meta = {
  title: "Options/GitHubIssuesSection",
  render: (_args, { loaded }) => <GitHubIssuesSection {...loaded.world} />,
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const On: Story = {
  ...optionsWorld(async (s, w) => {
    await w.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "bug", new_issues_need_me: true });
    await s.reportIssueWatcher("acme", { state: "watching" });
  }),
};
export const Off: Story = { ...optionsWorld() };
