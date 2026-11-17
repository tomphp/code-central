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
    await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "bug", new_issues_need_me: true });
    await s.reportIssueWatcher("acme", { state: "watching" });
  }),
};
export const SeveralRepositories: Story = {
  ...optionsWorld(async (s, w) => {
    await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web", "acme/api"], label: "", new_issues_need_me: false });
    await s.reportIssueWatcher("acme", {
      state: "error",
      problem: "repository_unreadable",
      message: "acme/api: gh can't read the repository acme/api",
      fix: "acme/api: Check the repository is the owner/name GitHub shows.",
      repositories: [
        {
          repository: "acme/api",
          problem: "repository_unreadable",
          message: "gh can't read the repository acme/api",
          fix: "Check the repository is the owner/name GitHub shows.",
        },
      ],
    });
  }),
};
export const Off: Story = { ...optionsWorld() };
