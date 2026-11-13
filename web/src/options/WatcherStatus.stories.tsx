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
