import type { Meta, StoryObj } from "@storybook/react-vite";
import type { NeedsMeRow } from "../api/types";
import { storyLink } from "../ticket/storyTickets";
import { NeedsMeLink } from "./NeedsMeLink";

const now = new Date("2026-11-13T12:00:00Z");
const since = "2026-11-13T09:00:00Z";

const ticket: NeedsMeRow = {
  project: "code-central",
  id: "code-central-492",
  title: "Sign in with a token",
  state: "In Review",
  reason: "in_review",
  since,
  focus_item: true,
};

const meta = {
  title: "Inbox/NeedsMeLink",
  component: NeedsMeLink,
  args: { row: ticket, now, link: storyLink("code-central-492") },
} satisfies Meta<typeof NeedsMeLink>;
export default meta;
type Story = StoryObj<typeof meta>;

// A Ticket waiting on the human.
export const Plain: Story = {};

// A Ticket of a Requirement names it.
export const WithRequirement: Story = {
  args: {
    row: { ...ticket, requirement: { spec: "code-central-491", title: "Reusable UI components" } },
  },
};

// A Project's item with no Ticket to open: not a link.
export const NoTicket: Story = {
  args: { row: { ...ticket, id: "", title: "Base diverged", reason: "base_diverged" } },
};
