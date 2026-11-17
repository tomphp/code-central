import type { Meta, StoryObj } from "@storybook/react-vite";
import { TicketRepositories } from "./TicketRepositories";

const meta = { title: "Ticket/TicketRepositories", component: TicketRepositories } satisfies Meta<typeof TicketRepositories>;
export default meta;
type Story = StoryObj<typeof meta>;

// In Progress: the Repositories the Factory reports changed so far.
export const Live: Story = {
  args: { ticket: { state: "In Progress", repositories: [{ name: "api", branch: "acme-7", commit: "4f1c2d9e8a7b" }] } },
};

// Accepted, landed in one Repository and waiting in the other, with a
// pull request and a CI result.
export const Settled: Story = {
  args: {
    ticket: {
      state: "Accepted",
      repositories: [
        {
          name: "acme",
          branch: "acme-7",
          commit: "9a8b7c6d5e4f",
          ci_result: "passed",
          ci_commit: "9a8b7c6d5e4f",
          pull_request: { number: 12, link: "https://github.com/acme/acme/pull/12", state: "open" },
        },
        { name: "api", branch: "acme-7", commit: "4f1c2d9e8a7b", landed: { commit: "1234567890ab", time: "2026-11-16T12:00:00Z" } },
      ],
    },
  },
};
