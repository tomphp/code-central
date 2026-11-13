import type { Meta, StoryObj } from "@storybook/react-vite";
import { GitHubIssuesSection } from "./GitHubIssuesSection";
import { storyApi, storyProject } from "./storyFixtures";

const meta = {
  title: "Options/GitHubIssuesSection",
  component: GitHubIssuesSection,
  args: { api: storyApi(), project: storyProject() },
} satisfies Meta<typeof GitHubIssuesSection>;
export default meta;
type Story = StoryObj<typeof meta>;

export const On: Story = {};
export const Off: Story = {
  args: {
    project: storyProject({
      github_issues: {
        on: false,
        repository: "",
        label: "",
        new_issues_need_me: false,
        watcher: { state: "not_reported" },
      },
    }),
  },
};
