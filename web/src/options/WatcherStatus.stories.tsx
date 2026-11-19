import type { Meta, StoryObj } from "@storybook/react-vite";
import { WatcherStatus } from "./WatcherStatus";

const meta = {
  title: "Options/WatcherStatus",
  component: WatcherStatus,
  args: { name: "CI Watcher", repository: "acme/web" },
} satisfies Meta<typeof WatcherStatus>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Watching: Story = { args: { watcher: { state: "watching" } } };
export const RateLimited: Story = {
  args: {
    watcher: {
      state: "watching",
      rate_limited_until: "2026-11-13T12:30:00Z",
    },
  },
};
export const Starting: Story = { args: { watcher: { state: "not_reported" } } };
export const WatchingOnAHost: Story = { args: { watcher: { state: "watching", host: "laptop" } } };
export const Elsewhere: Story = { args: { watcher: { state: "elsewhere", host: "desktop" } } };
export const Waiting: Story = {
  args: { watcher: { state: "waiting", message: "Waiting for a service to take the Watchers." } },
};
export const Error: Story = {
  args: {
    watcher: {
      state: "error",
      problem: "gh_missing",
      message: "gh isn't installed.",
      fix: "Install the GitHub CLI and run gh auth login.",
    },
  },
};
export const ErrorInSomeRepositories: Story = {
  args: {
    name: "Pull Request Watcher",
    repository: "its 3 Repositories",
    watcher: {
      state: "error",
      problem: "pat_missing",
      message: "2 Repositories fail: api: ADO_PAT isn't set; docs: gh can't read acme/docs",
      repositories: [
        { repository: "api", problem: "pat_missing", message: "ADO_PAT isn't set.", fix: "Set ADO_PAT for the service, then restart it." },
        { repository: "docs", problem: "repository_unreadable", message: "gh can't read acme/docs.", fix: "Check gh's account can see acme/docs." },
      ],
    },
  },
};
