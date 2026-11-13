import type { Meta, StoryObj } from "@storybook/react-vite";
import { PullRequestBadge } from "./PullRequestBadge";

const pr = { number: 42, link: "https://github.com/acme/widgets/pull/42" };

const meta = { title: "Ticket/PullRequestBadge", component: PullRequestBadge } satisfies Meta<typeof PullRequestBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Open: Story = { args: { pr: { ...pr, state: "open" } } };
export const Conflicting: Story = { args: { pr: { ...pr, state: "conflicting" } } };
export const Merged: Story = { args: { pr: { ...pr, state: "merged", merge_commit: "abc1234" } } };
export const Closed: Story = { args: { pr: { ...pr, state: "closed" } } };
// Inside a row that is itself a link, the badge opens the pull request with a click handler.
export const InRowLink: Story = { args: { pr: { ...pr, state: "open" }, inRowLink: true } };
// Shows nothing.
export const NoPullRequest: Story = {};
